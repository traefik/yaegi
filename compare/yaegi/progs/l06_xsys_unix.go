package main

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func main() {
	fmt.Println(unix.Getpid() > 0)
	var u unix.Utsname
	err := unix.Uname(&u)
	fmt.Println(err, unix.ByteSliceToString(u.Sysname[:]) != "")
	fmt.Println(unix.EINVAL.Error())
}
