//go:build goexperiment.jsonv2 && go1.27

package vm

import (
	"testing"

	"github.com/traefik/yaegi/v2/stdtypes"
)

// TestJSONTextGenericsPlugin calls jsontext.AppendQuote, AppendUnquote and
// AppendFormat from a package of a plugin that is not its main package:
// before, it stopped with "native func
// encoding/json/internal/jsonwire.AppendQuote not in registry".
func TestJSONTextGenericsPlugin(t *testing.T) {
	if !stdtypes.Available() {
		t.Skip("no embedded standard library types")
	}
	dir := writeModule(t, map[string]string{
		"go.mod": "module example.com/plug\n\ngo 1.27\n",
		"sub/sub.go": `package sub

import "encoding/json/jsontext"

type name string

func Q(s string) string {
	b, err := jsontext.AppendQuote([]byte("q:"), name(s))
	if err != nil {
		return err.Error()
	}
	u, err := jsontext.AppendUnquote(nil, b[2:])
	if err != nil {
		return err.Error()
	}
	f, err := jsontext.AppendFormat(nil, []byte(` + "`" + `{ "a" : 1 }` + "`" + `))
	if err != nil {
		return err.Error()
	}
	return string(b) + " " + string(u) + " " + string(f)
}
`,
		"plug.go": `package plug

import (
	"context"
	"fmt"
	"net/http"

	"example.com/plug/sub"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

func New(_ context.Context, next http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, sub.Q("a\"b"))
	}), nil
}
`,
	})
	want := `q:"a\"b" a"b {"a":1}`
	for _, goList := range []bool{false, true} {
		if got := serveOnce(t, Options{Dir: dir, GoList: goList}, "example.com/plug", "/"); got != want {
			t.Errorf("GoList %v: body %q, want %q", goList, got, want)
		}
	}
}

// TestJSONTextGenericsAlways checks that the generic functions of
// encoding/json/jsontext are always implemented by the VM: their source
// calls internal/jsonwire, which has no native symbols, so an instance that
// a loader built from source (the external tests of encoding/json) stopped
// with "jsonwire.AppendQuote not in registry".
func TestJSONTextGenericsAlways(t *testing.T) {
	for _, name := range []string{
		"encoding/json/jsontext.AppendQuote",
		"encoding/json/jsontext.AppendUnquote",
		"encoding/json/jsontext.AppendFormat",
	} {
		g, ok := genericImpls[name]
		if !ok || !g.always || g.make == nil {
			t.Errorf("%s: implemented %v, always %v", name, ok, g.always)
		}
	}
}
