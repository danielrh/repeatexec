package main
import (
   "os/exec"
   "os"
   "log"
   "flag"
   "syscall"
    )

func main() {
    gid := flag.Int("gid", os.Getgid(), "The user id of the user you wish to switch to")
    uid := flag.Int("uid", os.Getuid(), "The user id of the user you wish to switch to")
    flag.Parse()
    err := syscall.Setgid(*gid)
    if err != nil {
        log.Fatalf("Failed to switch to group id %d: %v\n", *gid, err)
        os.Exit(1)
    }
    err = syscall.Setuid(*uid)
    if err != nil {
        log.Fatalf("Failed to switch to user id %d: %v\n", *uid, err)
        os.Exit(1)
    }
    actual_uid := os.Getuid() 
    if actual_uid != *uid {
        log.Fatalf("Failed to switch to user id %d. Instead am %d\n", *uid, actual_uid)
        os.Exit(1)
    }
    actual_gid := os.Getgid()
    if actual_gid != *gid {
        log.Fatalf("Failed to switch to group id %d. Instead am %d\n", *gid, actual_gid)
        os.Exit(1)
    }
    args := flag.Args()
    if len(args) != 0 {
        cmd := exec.Command(args[0])
        cmd.Args = args;
        cmd.Stdin = os.Stdin
        cmd.Stdout = os.Stdout
        cmd.Stderr = os.Stderr
        cmd.Start()
        err := cmd.Wait()
        if err != nil {
            log.Fatal(err)
        }
    }    
}