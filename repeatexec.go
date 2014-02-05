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
    "strconv"
    "syscall"
)

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

type Instruction struct {
    Command        []string
    StdoutPipePath string
    StderrPipePath string
    StdinPipePath  string
    Gid            int
}

func accept_commands(command_prefix []string) {
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
    var instruction Instruction
    instruction_buffer := bufio.NewReader(os.Stdin)
    for {
        instruction_json, err := instruction_buffer.ReadBytes('\n')
        if err != nil {
            if err == io.EOF {
                return
            }
            log.Fatalf("Error on instruction stream %v\n", err)
        }
        json_err := json.Unmarshal(instruction_json, &instruction)
        if json_err == nil && len(instruction.Command) > 0 {
            syscall.Setgid(instruction.Gid)
            log.Print("Opening stdin: " + instruction.StdinPipePath)
            stdin_stream, err := os.Open(instruction.StdinPipePath)
            if err != nil {
                log.Fatal(err)
            }

            log.Print("Opening stdout: " + instruction.StdoutPipePath)
            stdout_stream, err := os.OpenFile(instruction.StdoutPipePath, os.O_WRONLY, 0)
            if err != nil {
                stdin_stream.Close()
                log.Fatal(err)
            }
            log.Print("Opening stderr: " + instruction.StderrPipePath)
            stderr_stream, err := os.OpenFile(instruction.StderrPipePath, os.O_WRONLY, 0)
            if err != nil {
                stdin_stream.Close()
                stdout_stream.Close()
                log.Fatal(err)
            }
            log.Print("Starting and waiting " + string(instruction_json))
            command := instruction.Command
            if len(command) > 0 && command[0] == "newuser" {
                uid += 1
                gid += 1
                if uid > max_uid {
                    io.WriteString(stdout_stream, "-1\n")
                    stderr_stream.Close()
                    stdout_stream.Close()
                    stdin_stream.Close()
                    log.Fatalf("uid %d is higher than the maximum (%d): restart...", uid, max_uid)
                }
                io.WriteString(stdout_stream, strconv.Itoa(uid)+"\n")
                stderr_stream.Close()
                stdout_stream.Close()
                stdin_stream.Close()
                null_byte := [1]byte{0}
                os.Stdout.Write(null_byte[:])
            } else {
                concatenated_command := concatenate_string_arrays(command_prefix, command)
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
                    stderr_stream.Close()
                    stdout_stream.Close()
                    stdin_stream.Close()
                    if instruction_buffer.Buffered() > 0 {
                        log.Fatalf("%d bytes in command buffer before exit code has been sent\n",
                            instruction_buffer.Buffered())
                    }
                    os.Stdout.Write(exit_code[:])
                }
            }
        } else {
            log.Printf("Error with instruction stream %s: %v", instruction_json, json_err)
        }
    }
}

func main() {
    if len(os.Args) > 1 && (os.Args[1] == "-version" || os.Args[1] == "--version") {
        fmt.Printf("%s\nCONFIGURED WITH\n", VERSION)
        var configuration_params = []string{
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
    var subcommand []string
    if len(RUNNER) != 0 {
        subcommand = make([]string, 1)
        subcommand[0] = RUNNER
        subcommand = concatenate_string_arrays(subcommand, os.Args[1:])
    } else {
        subcommand = os.Args[1:]
    }
    accept_commands(subcommand)
}
