package vm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestIntSizeConst checks that go/types uses the sizes of the platform of
// the VM: a constant that overflows a 32-bit int is a compile error on
// 386 and arm, like with gc, and is fine on a 64-bit platform. The go list
// loader used to take the platform of the toolchain (amd64 for a 386 VM
// built with GOARCH=386), so int had 64 bits there. The test sets GOARCH
// to another platform to check that go list does not take it.
func TestIntSizeConst(t *testing.T) {
	if intSize == 32 {
		t.Setenv("GOARCH", "amd64")
	} else {
		t.Setenv("GOARCH", "386")
	}
	dir := t.TempDir()
	src := "package main\n\nvar big int = 1 << 40\n\nfunc main() { println(big) }\n"
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := RunMain(RunOptions{Dir: dir}, filepath.Join(dir, "main.go"))
	if intSize == 32 {
		if err == nil || !strings.Contains(err.Error(), "overflows") {
			t.Fatalf("intSize 32: err = %v, want an overflow compile error", err)
		}
	} else if err != nil {
		t.Fatalf("intSize %d: %v", intSize, err)
	}
}
