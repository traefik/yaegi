package vm

import "golang.org/x/sys/unix"

// The requests that read and set the terminal attributes (xsys_test.go).
const termiosReqHost, termiosSetHost = unix.TIOCGETA, unix.TIOCSETA
