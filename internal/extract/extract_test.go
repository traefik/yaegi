package extract

import (
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"testing"
)

// check type checks the package path from src. deps are packages it may
// import, by path.
func check(t *testing.T, path, src string, deps map[string]*types.Package) *types.Package {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path+".go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	std := importer.Default()
	conf := types.Config{Importer: importerFunc(func(p string) (*types.Package, error) {
		if d, ok := deps[p]; ok {
			return d, nil
		}
		return std.Import(p)
	})}
	pkg, err := conf.Check(path, fset, []*ast.File{f}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return pkg
}

type importerFunc func(string) (*types.Package, error)

func (f importerFunc) Import(p string) (*types.Package, error) { return f(p) }

// jsonV2 is a package like encoding/json/v2 of Go 1.27: the only method of
// Options takes a type of an internal package.
func jsonV2(t *testing.T, options string) *types.Package {
	internal := check(t, "encoding/json/internal", `package internal
type NotForPublicUse struct{}
`, nil)
	return check(t, "encoding/json/v2", `package json
import "encoding/json/internal"
type Marshaler interface{ MarshalJSON() ([]byte, error) }
`+options, map[string]*types.Package{"encoding/json/internal": internal})
}

func TestInternalWrapper(t *testing.T) {
	pkg := jsonV2(t, `type Options interface{ JSONOptions(internal.NotForPublicUse) }
`)
	out, err := Generate("encoding/json/v2", pkg, Config{Dest: "stdsyms", Table: "Symbols", Build: "go1.27"})
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	for _, gone := range []string{`"encoding/json/internal"`, "_encoding_json_v2_Options", `"_Options"`, "interface wrapper for Options"} {
		if strings.Contains(got, gone) {
			t.Errorf("%s is in the output:\n%s", gone, got)
		}
	}
	for _, kept := range []string{"// Code generated", "\n//go:build go1.27\n", `Symbols["encoding/json/v2/json"]`, `"Options":`, `"_Marshaler":`, "func (W _encoding_json_v2_Marshaler) MarshalJSON"} {
		if !strings.Contains(got, kept) {
			t.Errorf("%s is missing from the output:\n%s", kept, got)
		}
	}
}

func TestInternalWrapperEmbedded(t *testing.T) {
	// The wrapper of an interface with an internal type in one of its
	// methods is dropped, also when the interface embeds another one.
	pkg := jsonV2(t, `type Options interface{ JSONOptions(internal.NotForPublicUse) }
type MarshalerTo interface {
	Marshaler
	MarshalTo(*internal.NotForPublicUse) error
}
`)
	out, err := Generate("encoding/json/v2", pkg, Config{Dest: "stdsyms", Table: "Symbols"})
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	if strings.Contains(got, "internal") || strings.Contains(got, "_MarshalerTo") {
		t.Errorf("a wrapper with an internal type is in the output:\n%s", got)
	}
	if !strings.Contains(got, `"MarshalerTo":`) || !strings.Contains(got, `"_Marshaler":`) {
		t.Errorf("a type or a wrapper is missing from the output:\n%s", got)
	}
}

func TestTableAndRestricted(t *testing.T) {
	pkg := check(t, "os", `package os
func Exit(code int) {}
func Getpid() int { return 0 }
var Args []string
const Big = 1 << 70
const Pi = 3.25
const Name = "x"
type T[P any] struct{}
func G[P any]() {}
type Number interface{ ~int | ~float64 }
type Stringer interface {
	String() string
	Printf(format string, a ...any)
}
`, nil)
	out, err := Generate("os", pkg, Config{Dest: "stdsyms", Table: "Unrestricted"})
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	want := []string{
		`Unrestricted["os/os"] = map[string]reflect.Value{`,
		`"Exit":   reflect.ValueOf(osExit),`,          // restricted: the local version
		`"Getpid": reflect.ValueOf(os.Getpid),`,       // not restricted
		`"Args":   reflect.ValueOf(&os.Args).Elem(),`, // a variable
		`constant.MakeFromLiteral("1180591620717411303424", token.INT, 0)`,
		`constant.MakeFromLiteral("3.25", token.FLOAT, 0)`,
		`constant.MakeFromLiteral("\"x\"", token.STRING, 0)`,
		`"_Stringer": reflect.ValueOf((*_os_Stringer)(nil)),`,
		"if W.WString == nil {",
		"func (W _os_Stringer) Printf(format string, a ...any) {",
		"W.WPrintf(format, a...)",
	}
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("%s is missing from the output:\n%s", w, got)
		}
	}
	for _, gone := range []string{`"T"`, `"G"`, `"Number"`, "//go:build", "Symbols["} {
		if strings.Contains(got, gone) {
			t.Errorf("%s is in the output:\n%s", gone, got)
		}
	}
}

func ExampleGenerate() {
	pkg, err := importer.ForCompiler(token.NewFileSet(), "source", nil).Import("unicode/utf16")
	if err != nil {
		panic(err)
	}
	out, err := Generate("unicode/utf16", pkg, Config{Dest: "stdsyms", Table: "Symbols", Build: "go1.26"})
	if err != nil {
		panic(err)
	}
	fmt.Println(strings.SplitN(string(out), "\n", 4)[2])
	// Output: //go:build go1.26
}
