package main

import (
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// runJSONRules runs the json cycle rules on src as a file of package path,
// then checkJSONCycle.
func runJSONRules(t *testing.T, path, src string, usesV2, v2Converted bool) (*ast.File, error) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	pi := &pinfo{p: &packages.Package{PkgPath: path}, kind: kConvert}
	rw := &rewriter{g: &gen{fset: fset}, pi: pi, f: f}
	rw.jsonPtrKey()
	rw.jsonV2Cycle()
	return f, checkJSONCycle(path, usesV2, v2Converted, pi)
}

const jsonV1Src = `package json

func (pe ptrEncoder) encode(e *encodeState, v reflect.Value, opts encOpts) {
	ptr := v.Interface()
	_ = ptr
}
`

// jsonV2Src is the shape of the code of encoding/json/v2 after rewrite().
const jsonV2Src = `package json

func visitPointer(m *seenPointers, v reflect.Value) error {
	p := typedPointer{v.Type(), v.UnsafePointer(), sliceLen(v)}
	_ = p
	return nil
}

func leavePointer(m *seenPointers, v reflect.Value) {
	p := typedPointer{v.Type(), v.UnsafePointer(), sliceLen(v)}
	delete(*m, p)
}

func marshalValueAny(enc *jsontext.Encoder, val any, mo *jsonopts.Struct) error {
	switch val := aotvm.Unbox(val).(type) {
	case map[string]any:
		return marshalObjectAny(enc, val, mo)
	}
	v := newAddressableValue(reflect.TypeOf(val))
	v.Set(reflect.ValueOf(val))
	return nil
}

func makeInterfaceArshaler(t reflect.Type) *arshaler {
	fncs.marshal = func(enc *jsontext.Encoder, va addressableValue, mo *jsonopts.Struct) error {
		v := newAddressableValue(va.Elem().Type())
		v.Set(va.Elem())
		return nil
	}
	return &fncs
}

func marshalEncode(out *jsontext.Encoder, in any, mo *jsonopts.Struct) (err error) {
	v := reflect.ValueOf(in)
	forceAddr := v.Kind() != reflect.Pointer
	if forceAddr {
		v2 := reflect.New(v.Type())
		v2.Elem().Set(v)
		v = v2
	}
	return nil
}
`

// TestJSONCycleRules checks that aotgen applies the cycle check rules of
// encoding/json (v1: jsonPtrKey, v2: jsonV2Cycle), and stops with an error
// when a target of the rules is missing: a new Go version must not give an
// AOT copy without the protection.
func TestJSONCycleRules(t *testing.T) {
	if _, err := runJSONRules(t, "encoding/json", jsonV1Src, false, false); err != nil {
		t.Errorf("v1: %v", err)
	}
	f, err := runJSONRules(t, "encoding/json/v2", jsonV2Src, false, false)
	if err != nil {
		t.Errorf("v2: %v", err)
	}
	var calls []string
	ast.Inspect(f, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == vmName {
				calls = append(calls, sel.Sel.Name)
			}
		}
		return true
	})
	if got := strings.Join(calls, ","); got != "NoUnbox,AddrCopy,AddrCopy,AddrCopy" {
		t.Errorf("v2 rewrites: %s, want NoUnbox,AddrCopy,AddrCopy,AddrCopy", got)
	}
	var src strings.Builder
	printer.Fprint(&src, token.NewFileSet(), f.Decls[len(f.Decls)-1].(*ast.FuncDecl).Body)
	if !strings.Contains(src.String(), "v = aotvm.AddrCopy(v).Addr()") || strings.Contains(src.String(), "v2") {
		t.Errorf("marshalEncode not rewritten:\n%s", src.String())
	}
	// encoding/json that only wraps encoding/json/v2.
	if _, err := runJSONRules(t, "encoding/json", "package json\n", true, true); err != nil {
		t.Errorf("v1 API on v2: %v", err)
	}

	fails := []struct {
		name, path, src     string
		usesV2, v2Converted bool
	}{
		// the reviewed case: encode.go not built, nothing found
		{"no ptrEncoder", "encoding/json", "package json\n", false, false},
		{"ptrEncoder without the key", "encoding/json",
			strings.Replace(jsonV1Src, "ptr := v.Interface()", "ptr := v.UnsafePointer()", 1), false, false},
		{"v2 not copied", "encoding/json", "package json\n", true, false},
		{"v2 empty", "encoding/json/v2", "package json\n", false, false},
		{"v2 key changed", "encoding/json/v2",
			strings.ReplaceAll(jsonV2Src, "v.UnsafePointer()", "v.Interface()"), false, false},
		{"v2 Unbox gone", "encoding/json/v2",
			strings.Replace(jsonV2Src, "aotvm.Unbox(val)", "val", 1), false, false},
		{"v2 copy changed", "encoding/json/v2",
			strings.Replace(jsonV2Src, "v.Set(va.Elem())", "v.Set(va)", 1), false, false},
		{"v2 top-level copy changed", "encoding/json/v2",
			strings.Replace(jsonV2Src, "v2.Elem().Set(v)", "v2.Elem().Set(reflect.ValueOf(in))", 1), false, false},
	}
	for _, c := range fails {
		if _, err := runJSONRules(t, c.path, c.src, c.usesV2, c.v2Converted); err == nil {
			t.Errorf("%s: no error", c.name)
		}
	}
}
