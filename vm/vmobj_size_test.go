package vm

import (
	"testing"
	"unsafe"
)

// TestVMObjSize keeps vmObj in the 32-byte size class on 64-bit hosts:
// every AOT wrapper and every handle of an interpreted value holds one.
// A value of the host goes in iv (hostObj), not in a field of its own.
func TestVMObjSize(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("64-bit hosts only")
	}
	if n := unsafe.Sizeof(vmObj{}); n > 32 {
		t.Errorf("vmObj is %d bytes, want at most 32", n)
	}
}
