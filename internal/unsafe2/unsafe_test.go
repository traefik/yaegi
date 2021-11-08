package unsafe2_test

import (
	"reflect"
	"testing"

	"github.com/traefik/yaegi/internal/unsafe2"
)

func TestSwapFieldType(t *testing.T) {
	f := []reflect.StructField{
		{
			Name: "A",
			Type: reflect.TypeOf(int(0)),
		},
		{
			Name: "B",
			Type: reflect.PtrTo(unsafe2.DummyType),
		},
		{
			Name: "C",
			Type: reflect.TypeOf(int64(0)),
		},
	}
	typ := reflect.StructOf(f)
	ntyp := reflect.PtrTo(typ)

	unsafe2.SetFieldType(typ, 1, ntyp)

	if typ.Field(1).Type != ntyp {
		t.Fatalf("unexpected field type: want %s; got %s", ntyp, typ.Field(1).Type)
	}
}
