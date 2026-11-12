//go:build unix && !aix

package vm

import "syscall"

// msgDontwait is syscall.MSG_DONTWAIT (checkRecvfrom).
const msgDontwait = syscall.MSG_DONTWAIT
