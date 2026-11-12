package vm

import (
	"context"
	"errors"
	"go/token"
	"go/types"
	"strings"
	"testing"
)

// TestStubAssembly checks Options.StubAssembly: without it, a plugin that
// reaches functions without a Go body is refused at load time, with an
// error that names the option; with it, the plugin loads, a C library
// system call fails with ENOSYS, and a call of an assembly function panics
// at run time.
func TestStubAssembly(t *testing.T) {
	if !hasStdTable("syscall") {
		t.Skip("no syscall table on this platform")
	}
	const pkg = "github.com/traefik/yaegi/v2/vm/testdata/stubasmplug"
	_, err := Load(Options{Dir: "..", UseUnsafe: true}, pkg)
	var pe *PreflightError
	if !errors.As(err, &pe) {
		t.Fatalf("without StubAssembly: %v, want a *PreflightError", err)
	}
	for _, want := range []string{"stubasmplug.syscall_syscall6 not in registry", "stubasmplug.addAsm not in registry", "Options.StubAssembly"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("without StubAssembly: %v\nwant %q", err, want)
		}
	}

	for _, opts := range []Options{
		{Dir: "..", UseUnsafe: true, StubAssembly: true},
		{Dir: "..", UseUnsafe: true, StubAssembly: true, AOT: true},
		{Dir: "..", Unrestricted: true, RealProcess: true, StubAssembly: true},
	} {
		p, err := Load(opts, pkg)
		if err != nil {
			t.Fatalf("%+v: %v", opts, err)
		}
		for _, name := range []string{"Probe", "Value"} {
			res, err := p.Call(context.Background(), name)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if v, _ := res[0].Interface(); v != true {
				t.Errorf("%s: %v, want true", name, v)
			}
		}
		_, err = p.Call(context.Background(), "Add")
		if err == nil || !strings.Contains(err.Error(), "addAsm has no Go body") || !strings.Contains(err.Error(), "Options.StubAssembly") {
			t.Errorf("Add: %v, want the stub to panic", err)
		}
	}
}

// TestLinknameRules checks what a //go:linkname to a runtime helper needs
// to be given: the file must import "unsafe" (as gc requires), and the
// function must have the signature of the runtime function. Otherwise the
// function stays refused at load time, or is a stub with StubAssembly.
func TestLinknameRules(t *testing.T) {
	const pkg = "github.com/traefik/yaegi/v2/vm/testdata/linkrulesplug"
	_, err := Load(Options{Dir: "..", UseUnsafe: true}, pkg)
	var pe *PreflightError
	if !errors.As(err, &pe) {
		t.Fatalf("%v, want a *PreflightError", err)
	}
	for _, want := range []string{"linkrulesplug.nanotime not in registry", "linkrulesplug.memmove not in registry"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%v\nwant %q", err, want)
		}
	}
	p, err := Load(Options{Dir: "..", UseUnsafe: true, StubAssembly: true}, pkg)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Nano", "Move"} {
		_, err := p.Call(context.Background(), name)
		if err == nil || !strings.Contains(err.Error(), "has no Go body") {
			t.Errorf("%s: %v, want the stub to panic", name, err)
		}
	}
}

// TestMemmoveChecks checks the refusals of runtime.memmove (linkname.go):
// a copy past the bytes the VM knows, and cells of other values.
func TestMemmoveChecks(t *testing.T) {
	m := NewMachine(nil, nil)
	src := []byte("hello")
	dst := make([]byte, 3)
	// the memory of the pointers, as unsafe.SliceData records it
	m.extents.addBytes(&src[0], len(src))
	m.extents.addBytes(&dst[0], len(dst))
	m.memmove(Value{R: &dst[0]}, Value{R: &src[0]}, 3)
	if string(dst) != "hel" {
		t.Fatalf("memmove: %q", dst)
	}
	for _, c := range []struct {
		name     string
		dst, src Value
		n        uint64
		want     string
	}{
		{"past the end", Value{R: &dst[0]}, Value{R: &src[0]}, 4, "reach outside the memory known to the VM"},
		{"cells", Value{R: &Value{}}, Value{R: &src[0]}, 1, "can only copy the bytes"},
		{"nil", Value{}, Value{R: &src[0]}, 1, "nil pointer"},
	} {
		func() {
			defer func() {
				if r := recover(); r == nil || !strings.Contains(errString(r), c.want) {
					t.Errorf("%s: panic %v, want %q", c.name, r, c.want)
				}
			}()
			m.memmove(c.dst, c.src, c.n)
		}()
	}
}

// TestMemmoveTypedChecks checks memmoveTyped (linkname.go), runtime.memmove
// when the SSA shows the type T behind both pointers: a copy of the whole
// value is the assignment, also for a T held in cells (a struct with a
// string); a copy of a part of such a value still goes to memmove, which
// refuses cells; a nil pointer is a nil dereference.
func TestMemmoveTypedChecks(t *testing.T) {
	m := NewMachine(nil, nil)
	T := types.NewStruct([]*types.Var{
		types.NewField(token.NoPos, nil, "A", types.Typ[types.Int], false),
		types.NewField(token.NoPos, nil, "B", types.Typ[types.String], false),
	}, nil)
	size := gcSizes.Sizeof(T)
	h := runtimeLinks["runtime.memmove"]
	src := &Value{R: []Value{{N: 1}, {R: "x"}}}
	dst := &Value{R: []Value{{}, {}}}
	m.memmoveTyped([]Value{{R: dst}, {R: src}, {N: size}}, T, h)
	if f := dst.R.([]Value); f[0].N != 1 || f[1].R != "x" {
		t.Fatalf("whole value: %v", f)
	}
	src.R.([]Value)[1].R = "y"
	if f := dst.R.([]Value); f[1].R != "x" {
		t.Fatalf("the copy shares the cells of the source: %v", f)
	}
	for _, c := range []struct {
		name     string
		dst, src Value
		n        int64
		want     string
	}{
		{"part", Value{R: dst}, Value{R: src}, 8, "can only copy the bytes"},
		{"larger", Value{R: dst}, Value{R: src}, size + 8, "can only copy the bytes"},
		{"nil", Value{}, Value{R: src}, size, "nil pointer"},
	} {
		func() {
			defer func() {
				if r := recover(); r == nil || !strings.Contains(errString(r), c.want) {
					t.Errorf("%s: panic %v, want %q", c.name, r, c.want)
				}
			}()
			m.memmoveTyped([]Value{c.dst, c.src, {N: c.n}}, T, h)
		}()
	}
}
