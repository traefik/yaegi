package vm

import (
	"strconv"
	"unsafe"
)

// The VM runs code for the platform it was built for: go/types uses the
// sizes of runtime.GOARCH (srcload.go, run.go) and native calls use the
// host ABI. So int, uint and uintptr have the size they have in the VM
// itself: 32 bits on 386, arm, mips and mipsle, 64 bits elsewhere.
//
// Values live in an int64 (Value.N). On a 32-bit platform, an int result
// is cut back to 32 bits like an int32 (truncKind). These are constants:
// on a 64-bit platform the compiler removes the extra cases, the int ops
// stay the same.
const (
	intSize = strconv.IntSize                  // bits of int, uint and uintptr
	intIs32 = intSize == 32                    // int is cut to 32 bits
	ptrSize = int64(unsafe.Sizeof(uintptr(0))) // bytes of a word (a pointer, len or cap)
)
