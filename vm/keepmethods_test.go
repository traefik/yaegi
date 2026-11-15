package vm

import (
	"context"
	"runtime"
	"testing"

	"github.com/traefik/yaegi/v2/internal/gorelease"
)

// TestKeepMethodsStaleAOT checks that native fmt and encoding/json keep the
// methods of the plugin types inside their values when the AOT copies
// cannot run (StaleAOTFallback after a Go patch release, keepmethods.go):
// json.Marshal([]any{Color(3)}) gave [3] and fmt printed {1 myerr 7} for
// a struct with a field of a type with a String method, with no error,
// where Go gives ["c3"] and {WARN myerr 7}. The wants are the output of
// `go run`.
func TestKeepMethodsStaleAOT(t *testing.T) {
	if aotPkg("encoding/json") == nil || aotPkg("fmt") == nil {
		t.Skip("no AOT copies of fmt and encoding/json in this build")
	}
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		"go.mod": "module example.com/plug\n\ngo 1.26\n",
		"plug.go": `package plug

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Color int

func (c Color) MarshalText() ([]byte, error) { return []byte(fmt.Sprintf("c<%d>", int(c))), nil }

type Inner struct{ C Color }

type Level int

func (l Level) String() string { return fmt.Sprint("L", 10-int(l)) }

type E struct {
	L   Level
	Msg string
	N   int
}

type PC int

func (c *PC) MarshalJSON() ([]byte, error) { return []byte(` + "`" + `"pc"` + "`" + `), nil }

type PK int

func (c *PK) MarshalText() ([]byte, error) { return []byte("pk"), nil }

type SM struct{ M map[string]PC }

type Z struct{ A int }

func (z Z) IsZero() bool                 { return z.A == 1 }
func (z Z) MarshalJSON() ([]byte, error) { return []byte(` + "`" + `"z"` + "`" + `), nil }

type F int

func (F) Format(s fmt.State, v rune) { fmt.Fprintf(s, "F(%c)", v) }

type O struct {
	A Color  ` + "`" + `json:"a,omitempty"` + "`" + `
	B Color  ` + "`" + `json:"b,omitzero"` + "`" + `
	C Z      ` + "`" + `json:"c,omitzero"` + "`" + `
	D *Color ` + "`" + `json:"d,omitempty"` + "`" + `
	E Color  ` + "`" + `json:"-"` + "`" + `
	f Color
	G []PC
	H PC
	I map[string]Color
}

type myErr struct {
	C Color
	L Level
}

func (myErr) Error() string { return "myerr" }

type W struct {
	Err error
	Lv  fmt.Stringer
}

type T struct {
	C    Color
	Kids []T ` + "`" + `json:",omitempty"` + "`" + `
	P    *T  ` + "`" + `json:",omitempty"` + "`" + `
}

var marshalers = map[string]func(any) ([]byte, error){"m": json.Marshal}

func js(v any) string {
	b, err := json.Marshal(v)
	return string(b) + " " + fmt.Sprint(err)
}

func JSON() string {
	var x any = Inner{C: 3}
	var w strings.Builder
	enc := json.NewEncoder(&w)
	enc.SetEscapeHTML(false)
	err := enc.Encode([]any{Color(1)})
	var ienc interface{ Encode(any) error } = json.NewEncoder(&w)
	ierr := ienc.Encode([]any{Color(5)})
	fb, ferr := marshalers["m"]([]any{Color(6)})
	return strings.Join([]string{
		js([]any{Color(3)}),
		js(map[string]any{"x": Inner{C: 3}}),
		js(x),
		js(map[string]any{"k": Color(2)}),
		js(map[Color]int{2: 1, 1: 2}),
		js([]any{O{}, &O{A: 1, B: 2, C: Z{1}, D: new(Color), f: 3, G: []PC{1}, H: 1, I: map[string]Color{"k": 4}}}),
		js([]any{W{Err: myErr{1, 1}, Lv: Level(1)}}),
		js([]any{T{C: 1, Kids: []T{{C: 2}}, P: &T{P: &T{}}}}),
		js([]map[string]PC{{"k": 1}}),
		js(&SM{M: map[string]PC{"k": 1}}),
		js([]map[PK]int{{1: 1}}),
		strings.TrimSpace(w.String()) + " " + fmt.Sprint(err, ierr),
		string(fb) + " " + fmt.Sprint(ferr),
	}, "\n")
}

func Fmt() string {
	var x any = E{1, "myerr", 7}
	return strings.Join([]string{
		fmt.Sprintf("%v", E{1, "myerr", 7}),
		fmt.Sprintf("%v %+v", []Level{1, 2}, x),
		fmt.Sprint([]any{E{2, "m", 3}}, map[string]any{"a": Level(1)}),
		fmt.Sprint(map[Level]int{3: 1, 1: 2, 2: 3}),
		fmt.Sprintf("%v|%+v|%d|%s", []any{F(1)}, struct{ X F }{2}, []Level{1}, []Level{2}),
		fmt.Sprintf("%v %+v", W{Err: myErr{1, 1}, Lv: Level(1)}, W{Err: myErr{1, 1}, Lv: Level(1)}),
		fmt.Errorf("e: %v", struct{ L Level }{1}).Error(),
	}, "\n")
}
`,
	})
	want := map[string]string{
		"JSON": `["c\u003c3\u003e"] <nil>
{"x":{"C":"c\u003c3\u003e"}} <nil>
{"C":"c\u003c3\u003e"} <nil>
{"k":"c\u003c2\u003e"} <nil>
{"c\u003c1\u003e":2,"c\u003c2\u003e":1} <nil>
[{"c":"z","G":null,"H":0,"I":null},{"a":"c\u003c1\u003e","b":"c\u003c2\u003e","d":"c\u003c0\u003e","G":["pc"],"H":"pc","I":{"k":"c\u003c4\u003e"}}] <nil>
[{"Err":{"C":"c\u003c1\u003e","L":1},"Lv":1}] <nil>
[{"C":"c\u003c1\u003e","Kids":[{"C":"c\u003c2\u003e"}],"P":{"C":"c\u003c0\u003e","P":{"C":"c\u003c0\u003e"}}}] <nil>
[{"k":1}] <nil>
{"M":{"k":1}} <nil>
[{"1":1}] <nil>
["c<1>"]
["c\u003c5\u003e"] <nil> <nil>
["c\u003c6\u003e"] <nil>`,
		"Fmt": `{L9 myerr 7}
[L9 L8] {L:L9 Msg:myerr N:7}
[{L8 m 3}] map[a:L9]
map[L9:2 L8:3 L7:1]
[F(v)]|{X:F(v)}|[1]|[L8]
{myerr L9} {Err:myerr Lv:L9}
e: {L9}`,
	}
	rel := gorelease.Release(runtime.Version())
	for _, opts := range []Options{
		{AOT: true, StaleAOTFallback: true}, // a newer Go patch release than the copies
		{},                                  // default mode, same
	} {
		opts.hooks = fakeAOT(t, rel+".98", rel+".99")
		opts.Dir = dir
		p, err := Load(opts, "example.com/plug")
		if err != nil {
			t.Fatalf("AOT %v: %v", opts.AOT, err)
		}
		if p.M.modeOf("encoding/json") != ModeNative || p.M.routed["encoding/json"] || p.M.routed["fmt"] {
			t.Fatalf("AOT %v: the calls go to the AOT copies, want native fmt and encoding/json", opts.AOT)
		}
		for name, w := range want {
			res, err := p.Call(context.Background(), name)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			got, err := res[0].Interface()
			if err != nil {
				t.Fatal(err)
			}
			if got != w {
				t.Errorf("AOT %v: %s() =\n%s\nwant\n%s", opts.AOT, name, got, w)
			}
		}
	}
}
