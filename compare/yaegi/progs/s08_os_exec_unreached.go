package main

import (
	"fmt"
	"os"
	"os/exec"
)

func never() { exec.Command("true").Run() }

func main() {
	if len(os.Args) > 5 {
		never()
	}
	fmt.Println("loaded")
}
