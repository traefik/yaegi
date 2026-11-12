package interp

import (
	"bytes"
	"io"
	"os"
	"reflect"
	"testing"
)

func TestExportClosureArg(t *testing.T) {
	outExp := []byte("0\n1\n2\n")
	// catch stdout
	backupStdout := os.Stdout
	defer func() {
		os.Stdout = backupStdout
	}()
	r, w, _ := os.Pipe()
	os.Stdout = w

	i := New(Options{})
	err := i.Use(Exports{
		"tmp/tmp": map[string]reflect.Value{
			"Func": reflect.ValueOf(func(s *[]func(), f func()) { *s = append(*s, f) }),
		},
	})
	if err != nil {
		t.Error(err)
	}
	i.ImportUsed()

	_, err = i.Eval(`
func main() {
	fs := []func(){}
	
	for i := 0; i < 3; i++ {
		i := i
		tmp.Func(&fs, func() { println(i) })
	}
	for _, f := range fs {
		f()
	}
}
`)
	if err != nil {
		t.Error(err)
	}
	// read stdout
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	outInterp, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(outInterp, outExp) {
		t.Errorf("\nGot: %q,\n want: %q", string(outInterp), string(outExp))
	}
}
