package main

// os/exec runs a program, as in Go. yaegi runs it at the real process level
// by default, and with -restricted -exec real (Options.Exec = ExecReal: os/exec
// alone, without the rest of RealProcess).

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

func main() {
	out, err := exec.Command("go", "env", "GOOS", "GOARCH").Output()
	fmt.Println(strings.Fields(string(out)), err)
	_, err = exec.Command("go", "no-such-command").Output()
	var ee *exec.ExitError
	fmt.Println(errors.As(err, &ee), ee != nil && ee.ExitCode() != 0)
	_, err = exec.LookPath("no-such-program-for-yaegi")
	fmt.Println(errors.Is(err, exec.ErrNotFound))
}
