package vm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/traefik/yaegi/v2/vm/testdata/natpkg"
)

const (
	natPkgPath  = "github.com/traefik/yaegi/v2/vm/testdata/natpkg"
	natPkgPlug  = "github.com/traefik/yaegi/v2/vm/testdata/natpkgplug"
	natPkgWant  = "info start; error failed: boom\na=1,b=2\n"
	natPkgHName = "natpkg"
)

type namer interface{ Name() string }

// namedHandler has ServeHTTP: AssertI must not give the handler behind the
// topHandler, whose ServeHTTP has no budget, cancel or recover.
type namedHandler interface {
	http.Handler
	namer
}

type namerObj struct{ Obj }

func (n namerObj) Name() string { return Out[string](n.Call("Name"), 0) }

func init() {
	RegisterNativePackage(&Package{
		Path:     natPkgPath,
		Funcs:    map[string]reflect.Value{"Use": reflect.ValueOf(natpkg.Use), "NewState": reflect.ValueOf(natpkg.NewState)},
		Types:    map[string]reflect.Type{"Sink": reflect.TypeFor[natpkg.Sink](), "State": reflect.TypeFor[natpkg.State]()},
		Vars:     map[string]reflect.Value{},
		Wrappers: map[string]reflect.Type{"Sink": reflect.TypeFor[natpkg.SinkWrapper]()},
	})
	RegisterWrapper[namer](func(o Obj) namer { return namerObj{o} })
}

// TestNativePackage runs a plugin that uses a package of the host in
// ModeNative (Traefik registers its own packages so). Before the fixes:
// the value of the plugin given as natpkg.Sink was boxed in errorW, the
// hand wrapper of error, because it has an Error method ("errorW is not
// assignable to natpkg.Sink"); the method String of the unexported member
// of natpkg.State was "not in registry"; and AssertI did not see the Name
// method of the handler behind the topHandler that the host gets.
func TestNativePackage(t *testing.T) {
	for _, aot := range []bool{false, true} {
		opts := Options{Dir: "..", AOT: aot, Modes: map[string]Mode{natPkgPath: ModeNative}}
		p, err := Load(opts, natPkgPlug)
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := createConfig(p)
		if err != nil {
			t.Fatal(err)
		}
		h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, natPkgHName)
		if err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
		if got := w.Body.String(); got != natPkgWant {
			t.Errorf("aot %v: got %q, want %q", aot, got, natPkgWant)
		}
		n, ok := AssertI[namer](h)
		if !ok {
			t.Fatalf("aot %v: the handler %T is not a namer", aot, h)
		}
		if got := n.Name(); got != natPkgHName {
			t.Errorf("aot %v: Name %q, want %q", aot, got, natPkgHName)
		}
		if _, ok := AssertI[namedHandler](h); ok {
			t.Errorf("aot %v: AssertI gave a namedHandler that skips the topHandler", aot)
		}
	}
}
