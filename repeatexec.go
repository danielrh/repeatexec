/////////////////////////////////////////////////////////////////////////////////////////////////
// Copyright (c) 2014, Daniel Reiter Horn
// All rights reserved.
//
// Redistribution and use in source and binary forms, with or without modification, are permitted
// provided that the following conditions are met:
//
// 1. Redistributions of source code must retain the above copyright notice, this list of 
//    conditions and the following disclaimer.
//
// 2. Redistributions in binary form must reproduce the above copyright notice, this list of
//    conditions and the following disclaimer in the documentation and/or other materials
//    provided with the distribution.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND ANY EXPRESS OR
// IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY
// AND FITNESS FOR A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR
// CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR
// CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
// SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
// THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR
// OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE
// POSSIBILITY OF SUCH DAMAGE.
//////////////////////////////////////////////////////////////////////////////////////////////////
package main

import (
    "bufio"
    "encoding/json"
    "fmt"
    "io"
    "log"
    "os"
    "os/exec"
    "path"
    "strconv"
    "syscall"
)

var PIPE_DIR = "/root"
var STDIN_PATH = "/stdin"
var STDOUT_PATH = "/stdout"
var STDERR_PATH = "/stderr"
var EXITCODE_PATH = "/exitcode"
var COMMAND_PATH = "/command"
var ABORT_PATH = "/abort"
var PERMISSIONS = "0600"
var STARTING_UID = "1000"
var STARTING_GID = "1000"
var MAX_UID = "1004"
var RUNNER = ""
var VERSION = "unknown"

func concatenate_string_arrays(s0, s1 []string) []string {
    retval := make([]string, len(s0)+len(s1))
    copy(retval, s0)
    copy(retval[len(s0):], s1)
    return retval
}

func check_stats(f *os.File, permissions os.FileMode) {
    stat, err := f.Stat()
    if err != nil {
        log.Fatalf("Error checking stats %v", err)
    }
    if (stat.Mode() & 0777) != permissions {
        log.Fatalf("Stats have been altered to %0d", stat.Mode())
    }
}

func accept_commands(command_path string, stdin_path string,
    stdout_path string, stderr_path string, exitcode_path string,
    permissions uint32, command_prefix []string) {
    uid, err := strconv.Atoi(STARTING_UID)
    if err != nil {
        log.Fatalf("Could not convert user id %s: %v", STARTING_UID, err)
    }
    gid, err := strconv.Atoi(STARTING_GID)
    if err != nil {
        log.Fatalf("Could not convert group id %s: %v", STARTING_GID, err)
    }
    max_uid, err := strconv.Atoi(MAX_UID)
    if err != nil {
        log.Fatalf("Could not convert user id %s: %v", MAX_UID, err)
    }
    var command []string
    for {
        command_stream, err := os.Open(command_path)
        if err != nil {
            log.Fatal(err)
        }
        command_buffer := bufio.NewReader(command_stream)
        command_json, err := command_buffer.ReadBytes('\n')
        json_err := json.Unmarshal(command_json, &command)
        if json_err == nil && len(command) > 0 {
            log.Print("Opening stdin: " + stdin_path)
            stdin_stream, err := os.Open(stdin_path)
            if err == nil {
                check_stats(stdin_stream, os.FileMode(permissions))
            }
            if err != nil {
                log.Fatal(err)
            }

            log.Print("Opening stdout: " + stdout_path)
            stdout_stream, err := os.OpenFile(stdout_path, os.O_WRONLY, 0)
            if err == nil {
                check_stats(stdout_stream, os.FileMode(permissions))
            }
            if err != nil {
                stdin_stream.Close()
                log.Fatal(err)
            }
            log.Print("Opening stderr: " + stderr_path)
            stderr_stream, err := os.OpenFile(stderr_path, os.O_WRONLY, 0)
            if err == nil {
                check_stats(stderr_stream, os.FileMode(permissions))
            }
            if err != nil {
                stdin_stream.Close()
                stdout_stream.Close()
                log.Fatal(err)
            }
            log.Print("Opening exitcode: " + exitcode_path)
            exitcode_stream, err := os.OpenFile(exitcode_path, os.O_WRONLY, 0)
            if err == nil {
                check_stats(exitcode_stream, os.FileMode(permissions))
            }
            if err != nil {
                stdin_stream.Close()
                stdout_stream.Close()
                stderr_stream.Close()
                log.Fatal(err)
            }
            log.Print("Starting and waiting " + string(command_json))
            if len(command) > 0 && command[0] == "newuser" {
                uid += 1
                gid += 1
                if uid > max_uid {
                    io.WriteString(stdout_stream, "-1\n")
                    log.Fatalf("uid %d is higher than the maximum allowed %d: restart...", uid, max_uid)
                }
                io.WriteString(stdout_stream, strconv.Itoa(uid)+"\n")
                null_byte := [1]byte{0}
                exitcode_stream.Write(null_byte[:])
            } else {

                concatenated_command := concatenate_string_arrays(command_prefix,
                    command)
                if len(concatenated_command) > 0 {
                    proc := exec.Command(concatenated_command[0])
                    proc.Args = concatenated_command
                    proc.Stdin = stdin_stream
                    proc.Stdout = stdout_stream
                    proc.Stderr = stderr_stream
                    var sys_proc_attr syscall.SysProcAttr
                    var cred syscall.Credential
                    cred.Uid = uint32(uid)
                    cred.Gid = uint32(gid)
                    cred.Groups = make([]uint32, 0)
                    sys_proc_attr.Credential = &cred
                    proc.SysProcAttr = &sys_proc_attr
                    proc.Start()
                    err = proc.Wait()
                    var exit_code [1]byte
                    if err != nil {
                        if proc.ProcessState.Success() {
                            exit_code[0] = 2
                        } else {
                            exit_code[0] = 1
                        }
                    } else {
                        exit_code[0] = 0
                    }
                    log.Printf("Process exited with error code %d", exit_code)
                    exitcode_stream.Write(exit_code[:])
                }
            }
            exitcode_stream.Close()
            stderr_stream.Close()
            stdout_stream.Close()
            stdin_stream.Close()
        }
        command_stream.Close()
        if err != nil {
            break
        }
    }
}

func abort() {
    pid := os.Getpid()
    log.Print(pid) //FIXME:
    _ = os.RemoveAll(STDIN_PATH)
    _ = os.RemoveAll(STDOUT_PATH)
    _ = os.RemoveAll(STDERR_PATH)
    _ = os.RemoveAll(COMMAND_PATH)
    _ = os.RemoveAll(EXITCODE_PATH)
    _ = os.RemoveAll(ABORT_PATH)
    os.Exit(0)
}

func main() {
    if len(os.Args) > 1 && (os.Args[1] == "-version" || os.Args[1] == "--version") {
        fmt.Printf("%s\nCONFIGURED WITH\n", VERSION)
        var configuration_params = []string{
            "pipe directory", PIPE_DIR,
            "command pipe", COMMAND_PATH,
            "stdin pipe", STDIN_PATH,
            "stdout pipe", STDOUT_PATH,
            "stderr pipe", STDERR_PATH,
            "abort pipe", ABORT_PATH,
            "pipe permissions", PERMISSIONS,
            "min uid", STARTING_UID,
            "min gid", STARTING_UID,
            "max uid", MAX_UID,
            "interim binary", RUNNER}
        for i, item := range configuration_params {
            fmt.Printf("%s", item)
            if i%2 == 0 {
                fmt.Printf(": ")
            } else {
                fmt.Printf("\n")
            }
        }
        os.Exit(0)
    }

    perm64, err := strconv.ParseUint(PERMISSIONS, 8, 32)
    permissions := uint32(perm64)
    if err != nil {
        log.Fatalf("Couldn't parse permissions %v", err)
    }
    STDIN_PATH = path.Join(PIPE_DIR, STDIN_PATH)
    STDOUT_PATH = path.Join(PIPE_DIR, STDOUT_PATH)
    STDERR_PATH = path.Join(PIPE_DIR, STDERR_PATH)
    EXITCODE_PATH = path.Join(PIPE_DIR, EXITCODE_PATH)
    ABORT_PATH = path.Join(PIPE_DIR, ABORT_PATH)
    COMMAND_PATH = path.Join(PIPE_DIR, COMMAND_PATH)
    _ = os.RemoveAll(COMMAND_PATH)
    _ = os.RemoveAll(STDIN_PATH)
    _ = os.RemoveAll(STDOUT_PATH)
    _ = os.RemoveAll(STDERR_PATH)
    _ = os.RemoveAll(EXITCODE_PATH)
    _ = os.RemoveAll(ABORT_PATH)
    err = syscall.Mkfifo(ABORT_PATH, permissions)
    if err != nil {
        log.Fatalf("Abort file exists %v", err)
    }
    os.Chmod(ABORT_PATH, os.FileMode(permissions))
    err = syscall.Mkfifo(EXITCODE_PATH, permissions)
    if err != nil {
        log.Fatalf("Exitcode file exists %v", err)
    }
    os.Chmod(EXITCODE_PATH, os.FileMode(permissions))
    err = syscall.Mkfifo(STDERR_PATH, permissions)
    if err != nil {
        log.Fatalf("Stderr file exists %v", err)
    }
    os.Chmod(STDERR_PATH, os.FileMode(permissions))
    err = syscall.Mkfifo(STDOUT_PATH, permissions)
    if err != nil {
        log.Fatalf("Stdout file exists %v", err)
    }
    os.Chmod(STDOUT_PATH, os.FileMode(permissions))
    err = syscall.Mkfifo(STDIN_PATH, permissions)
    if err != nil {
        log.Fatalf("Stdin file exists %v", err)
    }
    os.Chmod(STDIN_PATH, os.FileMode(permissions))
    err = syscall.Mkfifo(COMMAND_PATH, permissions)
    if err != nil {
        log.Fatalf("Command file exists %v", err)
    }
    os.Chmod(COMMAND_PATH, os.FileMode(permissions))
    var subcommand []string
    if len(RUNNER) != 0 {
        subcommand = make([]string, 1)
        subcommand[0] = RUNNER
        subcommand = concatenate_string_arrays(subcommand, os.Args[1:])
    } else {
        subcommand = os.Args[1:]
    }
    io.WriteString(os.Stdout, "ok\n")
    go accept_commands(COMMAND_PATH, STDIN_PATH, STDOUT_PATH, STDERR_PATH, EXITCODE_PATH,
        permissions, subcommand)
    for {
        // multiple people might hold abort open -- we need someone to write a byte
        f, err := os.Open(ABORT_PATH)
        if err == nil {
            var aborted [1]byte
            _, _ = f.Read(aborted[:])
            abort()
            f.Close()
        } else {
            log.Fatalf("Reopening abort %v\n", err)
            abort()
        }
    }
}
