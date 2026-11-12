package vm

import (
	"context"
	"go/types"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// softPlugin has only soft type errors: an unused variable, an unused
// import, an unused label, and := without a new variable. gc refuses it;
// yaegi runs it (it reports none of the unused names).
const softPlugin = `package p

import (
	"context"
	"net/http"
	"strings"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	unused := 1
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	L:
		x := "a"
		x := x + "b"
		w.Write([]byte(x))
	}), nil
}
`

// TestIgnoreSoftErrors checks Options.IgnoreSoftErrors with both loaders:
// without it, the soft errors refuse the plugin; with it, the plugin loads
// and runs as if the names were used. A hard type error still refuses it.
func TestIgnoreSoftErrors(t *testing.T) {
	needSource(t)
	dir, pkgPath := writePlugin(t, softPlugin)
	for _, golist := range []bool{false, true} {
		name := "default"
		if golist {
			name = "golist"
		}
		t.Run(name, func(t *testing.T) {
			_, err := Load(Options{Dir: dir, GoList: golist}, pkgPath)
			if err == nil || (!golist && !strings.Contains(err.Error(), "declared and not used")) {
				t.Fatalf("without IgnoreSoftErrors: %v, want the unused errors", err)
			}
			p, err := Load(Options{Dir: dir, GoList: golist, IgnoreSoftErrors: true}, pkgPath)
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := createConfig(p)
			if err != nil {
				t.Fatal(err)
			}
			h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "soft")
			if err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
			if got := w.Body.String(); got != "ab" {
				t.Errorf("body %q, want %q", got, "ab")
			}

			hard, hardPath := writePlugin(t, strings.Replace(softPlugin, `x + "b"`, `x + 1`, 1))
			if _, err := Load(Options{Dir: hard, GoList: golist, IgnoreSoftErrors: true}, hardPath); err == nil {
				t.Errorf("hard type error: %v, want a load error", err)
			}

			// Soft errors that are not about unused names: the program is
			// not valid Go, and Inc[string] would run "a" + 1.
			for name, src := range map[string]string{
				"type argument": "func Inc[T ~int](x T) T { return x + 1 }\n\nvar s = Inc[string](\"a\")\n",
				"constraint":    "type Num interface{ ~int | ~string }\n\nvar n Num\n",
			} {
				bad, badPath := writePlugin(t, softPlugin+"\n"+src)
				if _, err := Load(Options{Dir: bad, GoList: golist, IgnoreSoftErrors: true}, badPath); err == nil {
					t.Errorf("%s: loaded, want a load error", name)
				}
			}
		})
	}
}

// unusedExprPlugin has expression statements whose value is not used:
// "cpy.S (variable of type string) is not used" for go/types. gc refuses
// it; yaegi runs it. gc reports the slice expression at its "[" (column 4),
// go/types at "s" (column 3): the go list loader must still match them.
const unusedExprPlugin = `package p

import (
	"context"
	"net/http"
)

type Config struct{ S string }

func CreateConfig() *Config { return &Config{S: "a"} }

func New(_ context.Context, _ http.Handler, c *Config, _ string) (http.Handler, error) {
	cpy := *c
	cpy.S
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		s := cpy.S + "b"
		len(s)
		s == "ab"
		1
		"x" + s
		s[1:]
		w.Write([]byte(s))
	}), nil
}
`

// TestIgnoreUnusedExprs checks Options.IgnoreUnusedExprs with both
// loaders: without it, the plugin is refused as gc does; with it, the
// plugin loads and runs, and the values are dropped. The other errors of
// an expression statement still refuse it.
func TestIgnoreUnusedExprs(t *testing.T) {
	needSource(t)
	dir, pkgPath := writePlugin(t, unusedExprPlugin)
	for _, golist := range []bool{false, true} {
		name := "default"
		if golist {
			name = "golist"
		}
		t.Run(name, func(t *testing.T) {
			_, err := Load(Options{Dir: dir, GoList: golist, IgnoreSoftErrors: true}, pkgPath)
			if err == nil || (!golist && !strings.Contains(err.Error(), "cpy.S (variable of type string) is not used")) {
				t.Fatalf("without IgnoreUnusedExprs: %v, want the unused expression errors", err)
			}
			p, err := Load(Options{Dir: dir, GoList: golist, IgnoreUnusedExprs: true}, pkgPath)
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := createConfig(p)
			if err != nil {
				t.Fatal(err)
			}
			h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "unusedexpr")
			if err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
			if got := w.Body.String(); got != "ab" {
				t.Errorf("body %q, want %q", got, "ab")
			}

			// A built-in that is not called and a type used as an
			// expression are not valid statements either: still refused.
			for name, stmt := range map[string]string{
				"builtin": "len",
				"type":    "int",
			} {
				bad, badPath := writePlugin(t, strings.Replace(unusedExprPlugin, "\t\t1\n", "\t\t"+stmt+"\n", 1))
				if _, err := Load(Options{Dir: bad, GoList: golist, IgnoreSoftErrors: true, IgnoreUnusedExprs: true}, badPath); err == nil {
					t.Errorf("%s: loaded, want a load error", name)
				}
			}
		})
	}
}

// TestUnusedExprMsg checks the matcher of the "is not used" error of
// go/types: the modes of a checked value match, and nothing else matches
// (an invalid operand has no type information, even when the expression
// holds a string literal that looks like a mode and a type).
func TestUnusedExprMsg(t *testing.T) {
	for msg, want := range map[string]bool{
		"cpy.S (variable of type string) is not used":                                 true,
		"len(s) (value of type int) is not used":                                      true,
		"s == \"ab\" (untyped bool value) is not used":                                true,
		"1 (untyped int constant) is not used":                                        true,
		"\"x\" + s (value of type string) is not used":                                true,
		"1 << 100 (untyped int constant 1267650600228229401496703205376) is not used": true,
		"m[\"k\"] (map index expression of type int) is not used":                     true,
		"i.(int) (comma, ok expression of type int) is not used":                      true,
		"f (value of type func(int) (int, error)) is not used":                        true,
		"t.M (value of type func() int) is not used":                                  true,
		"http.StatusOK (untyped int constant 200) is not used":                        true,
		"x (invalid operand) is not used":                                             false,
		"\"a (value of type int) is not used\" + y (invalid operand) is not used":     false,
		"nil is not used":                                     false,
		"len (built-in) must be called":                       false,
		"int (type) is not an expression":                     false,
		"cannot use generic function F without instantiation": false,
		"declared and not used: x":                            false,
		"invalid operation: division by zero":                 false,
	} {
		if got := isUnusedExprErr(types.Error{Msg: msg}); got != want {
			t.Errorf("isUnusedExprErr(%q) = %v, want %v", msg, got, want)
		}
	}
	// A soft error never matches, whatever its message.
	if isUnusedExprErr(types.Error{Msg: "x (variable of type int) is not used", Soft: true}) {
		t.Error("a soft error matched")
	}
}
