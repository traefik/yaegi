package vm

import (
	"errors"
	"strings"
	"testing"

	"github.com/traefik/yaegi/v2/stdtypes"
)

// lostPlugin decodes a token with an inner NumericDate (golang-jwt), and
// decodes XML into a struct with an xml.Name and an inner UnmarshalText.
const lostPlugin = `package plug

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type NumericDate struct{ time.Time }

func (d *NumericDate) UnmarshalJSON(b []byte) error {
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	f, err := n.Float64()
	d.Time = time.Unix(int64(f), 0).UTC()
	return err
}

type Claims struct {
	Sub string       ` + "`json:\"sub\"`" + `
	Exp *NumericDate ` + "`json:\"exp\"`" + `
}

type Upper string

func (u *Upper) UnmarshalText(b []byte) error { *u = Upper(strings.ToUpper(string(b))); return nil }

type Doc struct {
	XMLName xml.Name ` + "`xml:\"doc\"`" + `
	Name    Upper    ` + "`xml:\"name\"`" + `
}

type Plain struct {
	XMLName xml.Name ` + "`xml:\"doc\"`" + `
	Name    string   ` + "`xml:\"name\"`" + `
}

// decode hides the type of the value from the load-time check.
func decode(b []byte, v any) error { return json.Unmarshal(b, v) }

func New(_ context.Context, next http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var c Claims
		switch r.URL.Path {
		case "/json":
			err := json.Unmarshal([]byte(` + "`{\"sub\":\"bob\",\"exp\":1700000000}`" + `), &c)
			fmt.Fprint(w, err, " ", c.Sub, " ", c.Exp != nil && c.Exp.Year() == 2023)
		case "/dynamic":
			err := decode([]byte(` + "`{\"sub\":\"bob\",\"exp\":1700000000}`" + `), &c)
			fmt.Fprint(w, err)
		case "/xml":
			var d Doc
			err := xml.Unmarshal([]byte("<doc><name>x</name></doc>"), &d)
			fmt.Fprint(w, err, " ", d.Name)
		case "/xmlstream":
			var d Doc
			err := xml.NewDecoder(strings.NewReader("<doc><name>x</name></doc>")).Decode(&d)
			fmt.Fprint(w, err, " ", d.Name)
		case "/plain":
			var p Plain
			err := xml.NewDecoder(strings.NewReader("<doc><name>x</name></doc>")).Decode(&p)
			fmt.Fprint(w, err, " ", p.Name)
		case "/accepted":
			accepted(w)
		}
	}), nil
}
// Values whose methods Go does not call either (encoding/json encode.go
// and decode.go): the check must accept them.

// Promoted has the MarshalJSON of its embedded Mark: json calls it and
// never looks inside the value, and Inner (a method on a value inside)
// does not count.
type Mark struct{ N int }

func (Mark) MarshalJSON() ([]byte, error) { return []byte(` + "`\"mark\"`" + `), nil }

type Inner struct{ V int }

func (Inner) MarshalJSON() ([]byte, error) { return []byte(` + "`\"inner\"`" + `), nil }

type Promoted struct {
	Mark
	I Inner
}

// Key is a map key: json writes a key of string kind as the string, and
// never calls MarshalJSON or UnmarshalJSON of a key.
type Key string

func (Key) MarshalJSON() ([]byte, error)     { return []byte(` + "`\"KEY\"`" + `), nil }
func (k *Key) UnmarshalJSON(b []byte) error { *k = "KEY"; return nil }

// Ambiguous embeds two marshalers: none is promoted, and the embedded
// structs are flattened, so none is called.
type Ambiguous struct {
	Mark
	Inner
}

// PtrRecv has its method on the pointer: not called in a value that is not
// addressable (a field of the value given to Marshal, a map element).
type PtrRecv struct{ N int }

func (*PtrRecv) MarshalJSON() ([]byte, error) { return []byte(` + "`\"ptr\"`" + `), nil }

type NotAddr struct {
	F PtrRecv
	M map[string]PtrRecv
	A [1]PtrRecv
	u PtrRecv
	S PtrRecv ` + "`json:\"-\"`" + `
}

func accepted(w http.ResponseWriter) {
	b1, err1 := json.Marshal(Promoted{Mark{1}, Inner{2}})
	b2, err2 := json.Marshal(map[Key]int{"k": 1})
	b3, err3 := json.Marshal(Ambiguous{Mark{1}, Inner{2}})
	b4, err4 := json.Marshal(NotAddr{F: PtrRecv{1}, M: map[string]PtrRecv{"a": {2}}, A: [1]PtrRecv{{3}}, u: PtrRecv{4}, S: PtrRecv{5}})
	var mk map[Key]int
	err5 := json.Unmarshal([]byte(` + "`{\"k\":1}`" + `), &mk)
	fmt.Fprint(w, string(b1), err1, " ", string(b2), err2, " ", string(b3), err3, " ", string(b4), err4, " ", mk, err5)
}
`

// TestLostMethods checks that a call of encoding/json or encoding/xml that
// would not call a method of an interpreted type inside its value is never
// silently wrong in default mode. Native json and xml cannot see these
// methods; the call goes to the AOT copy of the package when it can
// (aotroute.go). When it cannot (json or xml forced to native mode, a
// value that holds xml.Name, the xml stream methods), the load-time check fails the
// load when the type is known, and the call returns a *LostMethodError
// when it is not. Before, a golang-jwt token with "exp" was refused, with
// no error at load time.
func TestLostMethods(t *testing.T) {
	if !stdtypes.Available() {
		t.Skip("no embedded standard library types")
	}
	if len(AOTPackages()) == 0 {
		t.Skip("no AOT copy registered")
	}
	dir := writeModule(t, map[string]string{
		"go.mod":  "module example.com/plug\n\ngo 1.22\n",
		"plug.go": lostPlugin,
	})
	const pkg = "example.com/plug"

	// json and xml forced to native mode: no route to the AOT copies. (xml
	// is forced in every run but the AOT one: a plugin that imports
	// encoding/xml runs the standard library as its AOT copies when the
	// host has them, stdAOTModes, and the checks would have nothing to
	// refuse.)
	native := map[string]Mode{"encoding/json": ModeNative, "encoding/xml": ModeNative}
	xmlNative := map[string]Mode{"encoding/xml": ModeNative}
	_, err := Load(Options{Dir: dir, Modes: native}, pkg)
	var pe *PreflightError
	if !errors.As(err, &pe) {
		t.Fatalf("json native: error %v, want a *PreflightError", err)
	}
	for _, want := range []string{
		"plug.go:56:25:", "encoding/json.Unmarshal does not call the method plug.NumericDate.UnmarshalJSON",
		"the host sets the mode of encoding/json to native",
		"plug.go:63:24:", "encoding/xml.Unmarshal does not call the method plug.Upper.UnmarshalText",
		"the host sets the mode of encoding/xml to native",
		"plug.go:67:80:", "encoding/xml.Decoder.Decode does not call the method plug.Upper.UnmarshalText",
		"have no route to its AOT copy",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("json native: error\n%v\nwant %q", err, want)
		}
	}
	if len(pe.Problems) != 3 || strings.Contains(err.Error(), "Plain") {
		t.Errorf("json native: problems %q, want 3 (not the call with Plain, which has no method)", pe.Problems)
	}

	// Without the load-time check, the calls whose problem was reported
	// panic, and the call whose type was not known returns an error.
	opts := Options{Dir: dir, Modes: native, NoPreflight: true}
	got := serveOnce(t, opts, pkg, "/dynamic")
	want := "yaegi v2: in default mode, encoding/json.Unmarshal does not call the method plug.NumericDate.UnmarshalJSON, found in a value of type *plug.Claims"
	if !strings.HasPrefix(got, want) {
		t.Errorf("dynamic: body %q, want prefix %q", got, want)
	}
	if got := serveOnce(t, opts, pkg, "/plain"); got != "<nil> x" {
		t.Errorf("plain: body %q, want %q", got, "<nil> x")
	}
	// The calls whose methods Go does not call either are accepted (they
	// are not in the 3 problems above) and give the output of Go.
	const accepted = `"mark"<nil> {"k":1}<nil> {"N":1,"V":2}<nil> {"F":{"N":1},"M":{"a":{"N":2}},"A":[{"N":3}]}<nil> map[k:1] <nil>`
	if got := serveOnce(t, opts, pkg, "/accepted"); got != accepted {
		t.Errorf("accepted (json native): body %q, want %q", got, accepted)
	}

	// json in its default mode goes to its AOT copy, as in Go; the xml
	// calls (native) are reported.
	_, err = Load(Options{Dir: dir, Modes: xmlNative}, pkg)
	if !errors.As(err, &pe) {
		t.Fatalf("default: error %v, want a *PreflightError", err)
	}
	if len(pe.Problems) != 2 || strings.Contains(err.Error(), "encoding/json") {
		t.Errorf("default: problems %q, want the 2 xml calls", pe.Problems)
	}
	opts = Options{Dir: dir, Modes: xmlNative, NoPreflight: true}
	if got := serveOnce(t, opts, pkg, "/json"); got != "<nil> bob true" {
		t.Errorf("default json: body %q, want %q", got, "<nil> bob true")
	}
	if got := serveOnce(t, opts, pkg, "/dynamic"); got != "<nil>" {
		t.Errorf("default dynamic: body %q, want %q", got, "<nil>")
	}
	if got := serveOnce(t, opts, pkg, "/accepted"); got != accepted {
		t.Errorf("accepted (default): body %q, want %q", got, accepted)
	}

	// AOT mode: everything runs, as in Go.
	opts = Options{Dir: dir, AOT: true}
	for path, want := range map[string]string{"/json": "<nil> bob true", "/xml": "<nil> X", "/xmlstream": "<nil> X"} {
		if got := serveOnce(t, opts, pkg, path); got != want {
			t.Errorf("AOT %s: body %q, want %q", path, got, want)
		}
	}
}
