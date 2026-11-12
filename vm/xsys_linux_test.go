package vm

import (
	"testing"

	"golang.org/x/sys/unix"

	"github.com/traefik/yaegi/v2/stdtypes"
)

// The requests that read and set the terminal attributes (xsys_test.go).
const termiosReqHost, termiosSetHost = unix.TCGETS, unix.TCSETS

// TestSyscallUname checks syscall.Uname in the syscall subset of a
// restricted plugin with UseUnsafe (unsafesyscall.go): golang.org/x/sys/cpu
// calls it on linux/arm64, so wazero did not load ("native type
// syscall.Utsname not in registry").
func TestSyscallUname(t *testing.T) {
	if !hasStdTable("syscall") || !stdtypes.Available() {
		t.Skip("no syscall table or no embedded standard library types")
	}
	dir := writeModule(t, map[string]string{
		"go.mod": "module example.com/plug\n\ngo 1.22\n",
		"plug.go": `package plug

import "syscall"

func Sys() string {
	var u syscall.Utsname
	if err := syscall.Uname(&u); err != nil {
		return err.Error()
	}
	var b []byte
	for _, c := range u.Sysname {
		if c == 0 {
			break
		}
		b = append(b, byte(c))
	}
	return string(b)
}
`,
	})
	p, err := Load(Options{Dir: dir, UseUnsafe: true}, "example.com/plug")
	if err != nil {
		t.Fatal(err)
	}
	res, err := p.Call(t.Context(), "Sys")
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := res[0].Interface(); v != "Linux" {
		t.Errorf("Sys() = %v, want Linux", v)
	}
}
