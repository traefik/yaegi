package main

import (
	"fmt"
	"os/exec"
)

func main() {
	out, err := exec.Command("echo", "pwned").Output()
	fmt.Println(string(out), err)
}
