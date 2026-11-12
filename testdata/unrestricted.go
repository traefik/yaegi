// A program run without -restricted gets the standard library as in Go:
// os/exec and syscall have their symbols, log.Default is a *log.Logger, and
// os.Exit ends the program at once (no deferred call runs, no panic).
package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"syscall"
)

func main() {
	defer fmt.Println("deferred: must not run")

	out, err := exec.Command("sh", "-c", "echo from sh; exit 3").Output()
	var ee *exec.ExitError
	fmt.Printf("%q %v %v\n", out, errors.As(err, &ee), ee != nil && ee.ExitCode() == 3)

	fmt.Println(syscall.Getpagesize() > 0, syscall.Getpid() == os.Getpid())
	fmt.Printf("%T %T\n", log.Default(), log.New(os.Stdout, "", 0))

	os.Exit(0)
}
