package main

import (
	"fmt"
	"syscall"
)

func main() {
	fmt.Println(syscall.Getpid() > 0)
}
