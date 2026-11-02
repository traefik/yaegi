//go:build go1.27

package interp_test

import (
	"testing"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
)

func TestImportJSONv2(t *testing.T) {
	i := interp.New(interp.Options{})
	if err := i.Use(stdlib.Symbols); err != nil {
		t.Fatal(err)
	}

	eval(t, i, `import "encoding/json/v2"`)
	eval(t, i, `type T struct{}`)
	eval(t, i, `func (T) MarshalJSON() ([]byte, error) { return []byte("{}"), nil }`)

	// The package name must be json, not v2.
	eval(t, i, `var m json.Marshaler = T{}`)
	assertEval(t, i, `m != nil`, "", "true")
}
