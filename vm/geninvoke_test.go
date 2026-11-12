package vm

import (
	"context"
	"crypto"
	"log"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"net/rpc"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

// TestGenInvoke checks the generated direct calls of the methods of native
// values in an interface (genFastInvoke, natInvoke): results, errors, the
// allocations they save, and that a restricted machine still gets the
// replacement of a refused method ((*os.File).Chdir) through an interface
// of the plugin.
func TestGenInvoke(t *testing.T) {
	const src = `package p

import (
	"context"
	"fmt"
	"time"
)

func Err(c context.Context) error { return c.Err() }

func Nop(c context.Context) error { return nil }

func Deadline(c context.Context) (time.Time, bool) { return c.Deadline() }

func Str(s fmt.Stringer) string { return s.String() }

func Chdir(f interface{ Chdir() error }) error { return f.Chdir() }
`
	m, spkg := buildSrc(t, src)
	call := func(name string, args ...Value) Value {
		return m.Call(m.funcOf(spkg.Func(name)), args, nil)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if r := call("Err", m.natIface(ctx)); r.R != nil {
		t.Errorf("Err of a live context: %v", r.R)
	}
	cancel()
	r := call("Err", m.natIface(ctx))
	if iv, ok := r.R.(*Iface); !ok || natAny(Value{R: iv}) != context.Canceled {
		t.Errorf("Err of a canceled context: %#v, want context.Canceled", r.R)
	}
	dl := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	dctx, dcancel := context.WithDeadline(context.Background(), dl)
	defer dcancel()
	r = call("Deadline", m.natIface(dctx))
	tv, ok := r.R.([]Value)
	if !ok || len(tv) != 2 || tv[1].N != 1 || natAny(tv[0]).(time.Time) != dl {
		t.Errorf("Deadline: %#v", r.R)
	}
	if r := call("Str", m.natIface(1500*time.Millisecond)); r.R != "1.5s" {
		t.Errorf("Str: %#v, want 1.5s", r.R)
	}

	// The general path made allocations for Err (reflect.Call and its
	// slices); the direct call none: as many as a call that does nothing.
	bg := m.natIface(context.Background())
	allocs := func(name string) float64 {
		fn := m.funcOf(spkg.Func(name))
		n := math.Inf(1)
		for range 3 {
			n = min(n, testing.AllocsPerRun(100, func() { m.Call(fn, []Value{bg}, nil) }))
		}
		return n
	}
	n, base := allocs("Err"), allocs("Nop")
	if !raceEnabled && n > base {
		t.Errorf("Err through an interface: %.0f allocations, want %.0f (a call that does nothing)", n, base)
	}

	// A restricted machine keeps the refusal of (*os.File).Chdir.
	m.restricted = true
	dir := t.TempDir()
	f, err := os.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	func() {
		defer func() {
			rec := recover()
			if rec == nil || !strings.Contains(stringOf(rec), "not allowed") {
				t.Errorf("Chdir through an interface: recovered %v, want a refusal", rec)
			}
		}()
		call("Chdir", m.natIface(f))
	}()
	if now, _ := os.Getwd(); now != wd {
		t.Errorf("the directory changed to %s", now)
	}
}

func stringOf(r any) string {
	switch r := r.(type) {
	case error: // also *VMPanic
		return r.Error()
	case string:
		return r
	}
	return ""
}

// TestNatInvokeOK checks the guard of the direct calls (natInvokeOK) on a
// restricted machine: no direct call when the machine replaces the method
// of the type (refused, restricted or host method) or handles it in a
// special way (print methods).
func TestNatInvokeOK(t *testing.T) {
	m, _ := buildSrc(t, "package p\n")
	m.restricted = true
	for _, c := range []struct {
		t    reflect.Type
		name string
		want bool
	}{
		{reflect.TypeFor[*os.File](), "Chdir", false},     // refusedNative
		{reflect.TypeFor[*log.Logger](), "Fatal", false},  // restrictedMethods
		{reflect.TypeFor[*log.Logger](), "Print", false},  // printMethods
		{reflect.TypeFor[*http.Client](), "Do", false},    // hostMethod
		{reflect.TypeFor[*strings.Reader](), "Len", true}, // the real method
		{reflect.TypeFor[*os.File](), "Close", true},      // the real method
	} {
		if got := m.natInvokeOK(c.t, c.name); got != c.want {
			t.Errorf("natInvokeOK(%v, %s) = %v, want %v", c.t, c.name, got, c.want)
		}
	}
}

// TestGenInvokeReplaced checks that genFastInvoke has no direct call for
// the methods that a restricted machine replaces: refusedNative,
// restrictedMethods and the hostMethod types. natInvokeOK also refuses
// them at run time; this test keeps a change of natsigs_gen.go from
// exposing one of them with no test failing.
func TestGenInvokeReplaced(t *testing.T) {
	m, _ := buildSrc(t, "package p\n")
	m.restricted = true

	// The test helper reflectInvokeKey must give the keys of the generator.
	for _, c := range []struct {
		t    reflect.Type
		name string
	}{
		{reflect.TypeFor[*strings.Reader](), "Len"},
		{reflect.TypeFor[*os.File](), "Close"},
		{reflect.TypeFor[*net.TCPConn](), "SetDeadline"},
	} {
		meth, _ := c.t.MethodByName(c.name)
		if k := reflectInvokeKey(meth); m.genFastInvoke(k, nil) == nil {
			t.Errorf("%v.%s: no direct call for the key %q", c.t, c.name, k)
		}
	}

	keys := map[methodKey]bool{}
	for k := range refusedNative {
		keys[k] = true
	}
	for k := range refusedMethods() {
		keys[k] = true
	}
	for k := range restrictedMethods() {
		keys[k] = true
	}
	for _, typ := range []reflect.Type{
		reflect.TypeFor[*http.Server](),
		reflect.TypeFor[*http.Client](),
		reflect.TypeFor[*httptest.Server](),
		reflect.TypeFor[*rpc.Server](),
		reflect.TypeFor[crypto.Hash](),
	} {
		n := 0
		for i := 0; i < typ.NumMethod(); i++ {
			name := typ.Method(i).Name
			if _, ok := m.hostMethod(typ, name); ok {
				keys[methodKey{typ, name}] = true
				n++
			}
		}
		if n == 0 {
			t.Errorf("%v: no host method on a restricted machine", typ)
		}
	}
	for k := range keys {
		meth, ok := k.t.MethodByName(k.name)
		if !ok {
			t.Errorf("no method %v.%s", k.t, k.name)
			continue
		}
		if key := reflectInvokeKey(meth); m.genFastInvoke(key, nil) != nil {
			t.Errorf("%v.%s is replaced on a restricted machine, but genFastInvoke has a direct call for %q", k.t, k.name, key)
		}
	}
}

// reflectInvokeKey is invokeKey for the method meth of a native type (its
// receiver comes first in meth.Type). It writes the types that the
// generator supports as genTypeName does; the others never match a key.
func reflectInvokeKey(meth reflect.Method) string {
	ft := meth.Type
	var ps, rs []string
	for i := 1; i < ft.NumIn(); i++ {
		ps = append(ps, reflectTypeName(ft.In(i)))
	}
	for i := 0; i < ft.NumOut(); i++ {
		rs = append(rs, reflectTypeName(ft.Out(i)))
	}
	s := meth.Name + " func(" + strings.Join(ps, ", ") + ")"
	switch len(rs) {
	case 0:
	case 1:
		s += " " + rs[0]
	default:
		s += " (" + strings.Join(rs, ", ") + ")"
	}
	if ft.IsVariadic() {
		s += " ..."
	}
	return s
}

func reflectTypeName(t reflect.Type) string {
	if t.Name() != "" {
		if t.PkgPath() == "" {
			return t.Name() // predeclared: int, uint8 (byte), error...
		}
		return strings.NewReplacer("/", "_", ".", "_").Replace(t.PkgPath()) + "." + t.Name()
	}
	switch t.Kind() {
	case reflect.Pointer:
		return "*" + reflectTypeName(t.Elem())
	case reflect.Slice:
		return "[]" + reflectTypeName(t.Elem())
	}
	return t.String() // not supported by the generator
}
