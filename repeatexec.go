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
    "flag"
    "fmt"
    "io"
    "log"
    "os"
    "os/exec"
    "runtime"
    "strconv"
    "syscall"
)

var STARTING_UID = "1000"
var STARTING_GID = "1000"
var MAX_UID = "1004"

//The path the next binary should be invoked from
var RUNNER_PATH = "/usr/bin/"

//The types of binaries allowed to be selected by the user
var RUNNER0 = "strace"
var RUNNER1 = "strace+"
var RUNNERS = []string{RUNNER0, RUNNER1}

//The flag configuring the binary
var RUNNER_CONFIG_FLAG = "-e"

var RUNNER_ENVIRONMENT_FLAG = "-E"

var RUNNER_MEMORY_FLAG = "-O"

//the prefix to valid configuration flag
var RUNNER_CONFIG_PREFIX = "trace="

//a flag that will be passed into the runner every time
var RUNNER_ADDITIONAL_FLAG = "-f"

var VERSION = "unknown"

var ABORT_PIPE = "/pipes/abort"
var SHUTDOWN_PIPE = "/pipes/shutdown"
var FALLBACK_ABORT_PIPE = "/pipes/abort"
var FALLBACK_SHUTDOWN_PIPE = "/pipes/shutdown"

func concatenate_string_arrays(s0, s1 []string) []string {
    retval := make([]string, len(s0)+len(s1))
    copy(retval, s0)
    copy(retval[len(s0):], s1)
    return retval
}

func within(needle string, haystack []string) bool {
    for _, item := range haystack {
        if item == needle {
            return true
        }
    }
    return false
}

func isalphanumdashunder(s string) bool {
    for _, item := range s {
        item_is_alpha := (item >= 'A' && item <= 'Z') || (item >= 'a' && item <= 'z')
        item_is_numeric := (item >= '0' && item <= '9')
        item_is_underdash := (item == '_' || item == '-')
        if (!item_is_alpha) && (!item_is_numeric) && !item_is_underdash {
            return false
        }
    }
    return true
}

type Instruction struct {
    // The arguments to pass to exec
    Command []string
    // Whether, instead of running a new command, a new user should be switched to
    CreateNewUser bool
    // an alphanumeric (with dashes and underbars) config file that will be passed to the runner
    RunnerConfig string
    // sets environment variables to be set in the runner
    RunnerEnvironment map[string]string
    // sets a resource limit on the runner
    RunnerMemory int64
    // Runner to use to launch this command: must be in the array of allowed RUNNERS
    Runner string
    // The location of the stdin, stdout, and stderr pipes, used to communicate with the caller
    StdinPipePath  string
    StdoutPipePath string
    StderrPipePath string
    // group such that the above 3 pipes are openable: Stdout, Stderr for WRONLY, Stdin for RDONLYs
    Gid int
}

func read_shutdown(pipe_name string, pipe_name2 string, shutdown_channel chan<- int) {
    f, err := os.Open(pipe_name)
    if err != nil {
        f, err = os.Open(pipe_name2)
    }
    if err == nil {
        f.Close()
        shutdown_channel <- 0
    }
}
func read_instructions(instruction_channel chan<- []byte, shutdown_channel chan<- int) {
    instruction_buffer := bufio.NewReader(os.Stdin)
    for {
        instruction_json, err := instruction_buffer.ReadBytes('\n')
        if err != nil {
            if err == io.EOF {
                shutdown_channel <- 0
                return
            }
            log.Fatalf("Error on instruction stream %v\n", err)
        }
        instruction_channel <- instruction_json
    }
}

func wait_proc(proc *exec.Cmd, waited_channel chan<- error) {
    waited_channel <- proc.Wait()
}
func accept_commands() {
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
    proc_channel := make(chan error)
    instruction_channel := make(chan []byte)
    abort_channel := make(chan int)
    go read_shutdown(ABORT_PIPE, FALLBACK_ABORT_PIPE, abort_channel)
    shutdown_channel := make(chan int)
    go read_shutdown(SHUTDOWN_PIPE, FALLBACK_SHUTDOWN_PIPE, shutdown_channel)
    go read_instructions(instruction_channel, shutdown_channel)
    var instruction_json []byte
    for {
        select {
        case exit_code := <-shutdown_channel:
            os.Exit(exit_code)
            return
        case exit_code := <-abort_channel:
            os.Exit(exit_code)
            return
        case instruction_json = <-instruction_channel:
        }
        if err != nil {
            if err == io.EOF {
                return
            }
            log.Fatalf("Error on instruction stream %v\n", err)
        }
        var instruction Instruction

        json_err := json.Unmarshal(instruction_json, &instruction)
        if json_err == nil && (instruction.CreateNewUser || len(instruction.Command) > 0) {
            // this guarantees that the Setgid call applies to the same OSThread that will then
            // run os.Open on the stdin, stdout and stderr pipes.
            runtime.LockOSThread()
            syscall.Setgid(instruction.Gid)
            stdin_stream, err := os.Open(instruction.StdinPipePath)
            if err != nil {
                log.Fatal(err)
            }
            stdout_stream, err := os.OpenFile(instruction.StdoutPipePath, os.O_WRONLY, 0)
            if err != nil {
                stdin_stream.Close()
                log.Fatal(err)
            }
            stderr_stream, err := os.OpenFile(instruction.StderrPipePath, os.O_WRONLY, 0)
            if err != nil {
                stdin_stream.Close()
                stdout_stream.Close()
                log.Fatal(err)
            }
            runtime.UnlockOSThread()
            command := instruction.Command
            var exit_code [1]byte
            if instruction.CreateNewUser {
                if len(command) > 0 {
                    log.Print("Command %v ignored because we are creating a new user\n", command)
                }
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
            } else {
                if !within(instruction.Runner, RUNNERS) {
                    log.Fatalf("Using a disallowed runner %s, not within %v\n",
                        instruction.Runner,
                        RUNNERS)
                } else if !isalphanumdashunder(instruction.RunnerConfig) {
                    log.Fatalf("Using a disallowed configuration command: %s",
                        instruction.RunnerConfig)
                } else if len(instruction.RunnerConfig) == 0 {
                    log.Fatalf("Did not specify configuration to use with runner")
                } else {
                    command_prefix := []string{RUNNER_PATH + instruction.Runner,
                        RUNNER_CONFIG_FLAG,
                        RUNNER_CONFIG_PREFIX + instruction.RunnerConfig,
                        RUNNER_ADDITIONAL_FLAG}
                    if instruction.RunnerMemory > 0 {
                        command_prefix = concatenate_string_arrays(command_prefix,
                            []string{RUNNER_MEMORY_FLAG,
                                strconv.FormatInt(instruction.RunnerMemory, 10)})
                    }
                    for k,v := range instruction.RunnerEnvironment {
                        command_prefix = concatenate_string_arrays(command_prefix,
                            []string{RUNNER_ENVIRONMENT_FLAG, k + "=" + v})
                    }
                    concatenate_string_arrays(command_prefix, []string{"--"})
                    concatenated_command := concatenate_string_arrays(command_prefix, command)
                    log.Printf("PREFIX %v\n",concatenated_command)

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
                    go wait_proc(proc, proc_channel)
                    select {
                    case err = <-proc_channel:
                        if err != nil {
                            exit_code[0] = 1
                        } else {
                            exit_code[0] = 0
                        }
                    case exit_code := <-abort_channel:
                        os.Exit(exit_code)
                        return
                    }
                }
            }
            stderr_stream.Close()
            stdout_stream.Close()
            stdin_stream.Close()
            select {
            case bad_message := <-instruction_channel:
                log.Fatalf("%v bytes in command buffer before exit code has been sent\n",
                    bad_message)
            default:
                break
            }
            os.Stdout.Write(exit_code[:])
        } else {
            log.Printf("Error with instruction stream %s: %v", instruction_json, json_err)
        }
    }
}

func main() {
    runtime.GOMAXPROCS(1)
    version := flag.Bool("version", false, "Print out version information")
    flag.Parse()
    if *version {
        fmt.Printf("%s\nCONFIGURED WITH\n", VERSION)
        var configuration_params = []string{
            "min uid", STARTING_UID,
            "min gid", STARTING_UID,
            "max uid", MAX_UID,
            "runner config flag", RUNNER_CONFIG_FLAG,
            "runner config prefix", RUNNER_CONFIG_PREFIX,
            "runner additional flag", RUNNER_ADDITIONAL_FLAG}
        for i, item := range configuration_params {
            fmt.Printf("%s", item)
            if i%2 == 0 {
                fmt.Printf(": ")
            } else {
                fmt.Printf("\n")
            }
        }
        fmt.Printf("Configured Runners:\n")
        for _, runner := range RUNNERS {
            fmt.Printf("%s%s\n", RUNNER_PATH, runner)
        }
        os.Exit(0)
    }
    accept_commands()
}
