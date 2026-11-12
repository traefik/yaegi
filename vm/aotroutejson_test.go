//go:build goexperiment.jsonv2

package vm

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// TestJSONOptionsCopy checks the copy of the options of a native
// json.Decoder for the AOT copy of json/v2 (copyJSONOptions): the flags of
// UseNumber and DisallowUnknownFields are kept, and the options of
// DefaultOptionsV1 of the copy, a variable of the package, do not change.
func TestJSONOptionsCopy(t *testing.T) {
	p := aotPkg(jsonPath)
	if p == nil {
		t.Skip("no AOT copy of encoding/json")
	}
	defaults := p.Funcs["DefaultOptionsV1"]
	flags := func(o reflect.Value) [2]uint64 {
		f := o.Elem().Elem().FieldByName("Flags")
		return [2]uint64{f.Field(0).Uint(), f.Field(1).Uint()}
	}
	before := flags(defaults.Call(nil)[0])

	d := json.NewDecoder(strings.NewReader(""))
	d.UseNumber()
	d.DisallowUnknownFields()
	src := fieldOf(reflect.ValueOf(d).Elem(), "opts")
	dst := reflect.New(defaults.Type().Out(0)).Elem()
	dst.Set(defaults.Call(nil)[0])
	if !copyJSONOptions(dst, src) {
		t.Fatal("copyJSONOptions refused the options of a Decoder")
	}
	if got, want := flags(dst), flags(src); got != want {
		t.Errorf("flags of the copy %x, want %x", got, want)
	}
	if flags(dst) == before {
		t.Errorf("the flags of UseNumber and DisallowUnknownFields are lost")
	}
	if after := flags(defaults.Call(nil)[0]); after != before {
		t.Errorf("DefaultOptionsV1 of the copy changed: %x, was %x", after, before)
	}
}
