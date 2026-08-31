package interp

import (
	"reflect"
	"strings"
	"testing"
)

func TestGorneshRangeAssignmentRejectsInvalidTargetsAtCompileTime(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{name: "type mismatch", source: `var s string; for s = range []int{1} { break }`, want: "cannot use type int as type string"},
		{name: "literal", source: `for 1 = range []int{1} { break }`, want: "cannot assign to"},
		{name: "call", source: `func f() int { return 0 }; for f() = range []int{1} { break }`, want: "cannot assign to"},
		{name: "field of returned value", source: `type S struct{ X int }; func s() S { return S{} }; for s().X = range []int{1} { break }`, want: "cannot assign to"},
		{name: "index of returned array", source: `func a() [1]int { return [1]int{} }; for a()[0] = range []int{1} { break }`, want: "cannot assign to"},
		{name: "field of map element", source: `type S struct{ X int }; m := map[int]S{0: {}}; for _, m[0].X = range []int{1} { break }`, want: "cannot assign to"},
		{name: "field of returned map element", source: `type S struct{ X int }; func m() map[int]S { return map[int]S{0: {}} }; for _, m()[0].X = range []int{1} { break }`, want: "cannot assign to"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := New(Options{}).Eval(test.source)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Eval error = %v, want diagnostic containing %q", err, test.want)
			}
			if strings.Contains(err.Error(), "reflect:") || strings.Contains(err.Error(), "Panic") {
				t.Fatalf("invalid range assignment reached runtime: %v", err)
			}
		})
	}
}

func TestGorneshRangeAssignmentAcceptsParenthesizedTarget(t *testing.T) {
	value, err := New(Options{}).Eval(`var x int; for (x) = range []int{3} { break }; x`)
	if err != nil || value.Int() != 0 {
		t.Fatalf("parenthesized range assignment: value=%v err=%v", value, err)
	}
}

func TestGorneshRangeAssignmentEvaluatesExpressionTargetsPerIteration(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{name: "existing identifier", source: `i := 1; for i = range []int{7, 8} { break }; i == 0`},
		{name: "two phase index", source: `i := 0; a := [2]int{}; for i, a[i] = range []int{7, 8} { _ = i }; i == 1 && a[0] == 8 && a[1] == 0`},
		{name: "map key side effect", source: `i := 1; calls := 0; m := map[int]int{}; idx := func() int { calls++; return i }; for i, m[idx()] = range []int{7, 8} { _ = i }; i == 1 && calls == 2 && m[1] == 7 && m[0] == 8`},
		{name: "range value before map target", source: `a := []int{7}; m := map[int]int{}; idx := func() int { a[0] = 9; return 0 }; for _, m[idx()] = range a { break }; a[0] == 9 && m[0] == 7`},
		{name: "range value before pointer target", source: `a := []int{7}; out := 0; pointer := func() *int { a[0] = 9; return &out }; for _, *pointer() = range a { break }; a[0] == 9 && out == 7`},
		{name: "each target evaluated once", source: `trace := ""; x, y := 9, 9; pointer := func(label string, value *int) *int { trace += label; return value }; for *pointer("k", &x), *pointer("v", &y) = range []int{7} { break }; trace == "kv" && x == 0 && y == 7`},
		{name: "returned slice index", source: `var a = []int{0}; func s() []int { return a }; for _, s()[0] = range []int{9} { break }; a[0] == 9`},
		{name: "returned pointer field", source: `type S struct{ X int }; var p = &S{}; func s() *S { return p }; for _, s().X = range []int{7} { break }; p.X == 7`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value, err := New(Options{}).Eval(test.source)
			if err != nil || value.Interface() != true {
				t.Fatalf("range expression target: value=%v err=%v", value, err)
			}
		})
	}
}

func TestGorneshRangeAssignmentAcceptsBinaryPackageVariable(t *testing.T) {
	exported := 0
	i := New(Options{})
	if err := i.Use(Exports{"rangepkg/rangepkg": {
		"Value": reflect.ValueOf(&exported).Elem(),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`import "rangepkg"; for _, rangepkg.Value = range []int{9} { break }`); err != nil {
		t.Fatalf("range assignment to binary package variable: %v", err)
	}
	if exported != 9 {
		t.Fatalf("binary package variable = %d, want 9", exported)
	}
}
