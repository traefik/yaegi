package vm

import (
	"context"
	"testing"
)

// TestReflectFlagForge checks the flag field of a reflect.Value written with
// unsafe (go-spew's bypass.go): clearing the read-only bits and setting the
// addressable bit works on a value of the plugin, as in gc, and gives
// nothing on a value of a type of the host: Interface of its unexported
// field still panics, the field is never settable, and a byte of a string
// in read-only memory is not written.
func TestReflectFlagForge(t *testing.T) {
	for _, opts := range []Options{
		{Dir: "..", UseUnsafe: true},
		{Dir: "..", UseUnsafe: true, AOT: true},
	} {
		p, err := Load(opts, "github.com/traefik/yaegi/v2/vm/testdata/flagforgeplug")
		if err != nil {
			t.Fatalf("%+v: %v", opts, err)
		}
		res, err := p.Call(context.Background(), "Probe")
		if err != nil {
			t.Fatalf("%+v: %v", opts, err)
		}
		got, _ := res[0].Interface()
		want := "own: true true true ok; " +
			"field: false false false panic; set field: panic; " +
			"ptr field: false true false panic; set ptr field: panic; addr: ok; buffer: host; " +
			"byte: ok OK OK"
		if got != want {
			t.Errorf("%+v:\n got %q\nwant %q", opts, got, want)
		}
	}
}
