package vm

import (
	"context"
	"testing"
)

// TestReflectByteSharedRead checks that goroutines can read the same
// reflect.Value of a byte of a Go []byte at the same time, as in gc: a read
// must not write the cell that the goroutines share (a race under -race),
// and each read sees the byte as it is now.
func TestReflectByteSharedRead(t *testing.T) {
	for _, opts := range []Options{
		{Dir: ".."},
		{Dir: "..", AOT: true},
	} {
		p, err := Load(opts, "github.com/traefik/yaegi/v2/vm/testdata/reflectbyteplug")
		if err != nil {
			t.Fatalf("%+v: %v", opts, err)
		}
		res, err := p.Call(context.Background(), "Read")
		if err != nil {
			t.Fatalf("%+v: %v", opts, err)
		}
		got, _ := res[0].Interface()
		if want := 4000*int('e') + 7; got != want {
			t.Errorf("%+v: sum of the reads: %v, want %d", opts, got, want)
		}
	}
}
