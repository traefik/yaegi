package vm

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// shiftWide is F[T0, ..., Tn-1, U] calling F[P[int], T0, ..., Tn-2, S]
// with S a struct of fields fields.
func shiftWide(n, fields int) string {
	var params, shifted, ints, st strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&params, "T%d, ", i)
		if i < n-1 {
			fmt.Fprintf(&shifted, ", T%d", i)
		}
		ints.WriteString("int, ")
	}
	for i := 0; i < fields; i++ {
		fmt.Fprintf(&st, "f%d [%d]int; ", i, i+1)
	}
	return "package p\n\ntype P[A any] struct{ a A }\n" +
		fmt.Sprintf("func F[%sU any](k int) { if k > 0 { F[P[int]%s, struct{ %s}](k - 1) } }\n", params.String(), shifted.String(), st.String()) +
		fmt.Sprintf("func G() { F[%sint](1) }\n", ints.String())
}

// shiftSame is shiftWide with S[int] as the last type argument, where S[X]
// is a struct of fields fields of the same type X: after the first field,
// each field is in the memo.
func shiftSame(n, fields int) string {
	var params, shifted, ints, st strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&params, "T%d, ", i)
		if i < n-1 {
			fmt.Fprintf(&shifted, ", T%d", i)
		}
		ints.WriteString("int, ")
	}
	for i := 0; i < fields; i++ { // one field list: the source stays short
		fmt.Fprintf(&st, "f%d, ", i)
	}
	return "package p\n\ntype P[A any] struct{ a A }\n" +
		fmt.Sprintf("type S[X any] struct{ %s_ X }\n", st.String()) +
		fmt.Sprintf("func F[%sU any](k int) { if k > 0 { F[P[int]%s, S[int]](k - 1) } }\n", params.String(), shifted.String()) +
		fmt.Sprintf("func G() { F[%sint](1) }\n", ints.String())
}

// chain writes n declarations made by line(i), after head and before tail.
func chain(head string, n int, line func(i int) string, tail string) string {
	var b strings.Builder
	b.WriteString("package p\n\n" + head)
	for i := 1; i <= n; i++ {
		b.WriteString(line(i))
	}
	b.WriteString(tail)
	return b.String()
}

// loadBombs are small sources that made go/types, go/constant or the SSA
// builder run for minutes or allocate gigabytes (loadbomb.go), with a part
// of the error that refuses them.
var loadBombs = []struct{ name, src, want string }{
	{"genericAlias", chain("type P[A, B any] struct{ a A; b B }\ntype T0 = int\n", 40,
		func(i int) string { return fmt.Sprintf("type T%d = P[T%d, T%d]\n", i, i-1, i-1) }, "var X T40\n"), "type too large"},
	{"genericNamed", chain("type P[A, B any] struct{ a A; b B }\ntype T0 int\n", 200,
		func(i int) string { return fmt.Sprintf("type T%d P[T%d, T%d]\n", i, i-1, i-1) }, "var X T200\n"), "type too large"},
	{"structNamed", chain("type T0 int\n", 200,
		func(i int) string { return fmt.Sprintf("type T%d struct{ a, b T%d }\n", i, i-1) }, "var X T200\n"), "type too large"},
	{"structAlias", chain("type T0 = int\n", 60,
		func(i int) string { return fmt.Sprintf("type T%d = struct{ a, b T%d }\n", i, i-1) }, "var X T60\n"), "type too large"},
	{"funcAlias", chain("type T0 = int\n", 60,
		func(i int) string { return fmt.Sprintf("type T%d = func(T%d, T%d)\n", i, i-1, i-1) }, "var X T60\n"), "type too large"},
	{"structDeep", chain("type T0 int\n", 3000,
		func(i int) string { return fmt.Sprintf("type T%d struct{ a T%d; b, c, d int }\n", i, i-1) }, "var X T3000\n"), "nested too deep"},
	{"localTypes", chain("func F() any {\n\ttype T0 int\n", 200,
		func(i int) string { return fmt.Sprintf("\ttype T%d struct{ a, b T%d }\n", i, i-1) }, "\tvar x T200\n\treturn x\n}\n"), "type too large"},
	{"constDouble", chain("const S0 = \"xx\"\n", 40,
		func(i int) string { return fmt.Sprintf("const S%d = S%d + S%d\n", i, i-1, i-1) }, "func F() int { return len(S40) }\n"), "string constants"},
	{"constBlock", chain("const (\n\tS0 = \"xx\"\n", 40,
		func(i int) string { return fmt.Sprintf("\tS%d = S%d + S%d\n", i, i-1, i-1) }, ")\n"), "string constants"},
	// The scope of a local constant starts after its declaration: each a
	// is the a of the block around it, twice.
	{"constScopes", chain("func F() int {\n\tconst a = \"xx\"\n", 40,
		func(int) string { return "\t{ const a = a + a\n" }, "\t_ = a\n"+strings.Repeat("\t}\n", 40)+"\treturn 0\n}\n"), "string constants"},
	{"constExpr", chain("const S0 = \"xx\"\n", 25,
		func(i int) string { return fmt.Sprintf("const S%d = S%d + S%d\n", i, i-1, i-1) },
		"func F() int { return len(S25+S25+S25+S25+S25+S25+S25+S25+S25+S25+S25+S25+S25+S25+S25+S25) }\n"), "string constants"},
	{"genericFuncChain", chain("type P[A, B any] struct{ a A; b B }\n", 40,
		func(i int) string { return fmt.Sprintf("func F%d[T any]() any { return F%d[P[T, T]]() }\n", i, i+1) },
		"func F41[T any]() any { var x T; return x }\nfunc G() any { return F1[int]() }\n"), "generic instances of the plugin are too large"},
	// Sixteen copies of the argument per level: the fifth level passes the
	// size limit (about 1.07 million nodes) while the levels below it add up
	// to about 150000, below the work limit.
	{"genericFuncWide", chain("type P[A0, A1, A2, A3, A4, A5, A6, A7, A8, A9, A10, A11, A12, A13, A14, A15 any] struct{ a [0]A0 }\n", 10,
		func(i int) string {
			return fmt.Sprintf("func F%d[T any]() any { return F%d[P[%s]]() }\n", i, i+1, strings.TrimSuffix(strings.Repeat("T, ", 16), ", "))
		},
		"func F11[T any]() any { var x T; return x }\nfunc G() any { return F1[int]() }\n"), "generic instance too large to build"},
	// A self-recursive function that shifts 1000 type parameters, with a
	// struct of 5000 fields as the last argument: 1000 versions of the sizes
	// of the cycle, each walking the struct again (6.8 s and 3.2 GB with
	// 20000 fields when the measures were counted, not the types walked).
	{"genericShiftWide", shiftWide(1000, 5000), "cycle of generic declarations is too long to check"},
	// The same with fields of one type: the memo hits count too.
	// The source goes to a child process in an environment variable,
	// which Linux limits to 128 KB.
	{"genericShiftSame", shiftSame(1000, 8000), "cycle of generic declarations is too long to check"},
	// A chain of 400 generic functions, each wrapping its type argument once
	// (sizes grow linearly), declared callees first, and 8000 calls in the
	// last body: the SSA builder hashed and compared a type argument of 400
	// nodes for each call, 25 s. Each size is small; the sum is not.
	{"genericFuncDeep", "package p\n\ntype P[A any] struct{ a A }\nfunc H[T any]() {}\nfunc F400[T any]() {\n" +
		strings.Repeat("\tH[T]()\n", 8000) + "}\n" +
		chain("", 399, func(i int) string {
			return fmt.Sprintf("func F%d[T any]() { F%d[P[T]]() }\n", 400-i, 401-i)
		}, "func G() { F1[int]() }\n")[len("package p\n\n"):], "generic instances of the plugin are too large"},
	{"genericFuncFan", chain("type A[T any] struct{ x T }\ntype B[T any] struct{ x T }\n", 30,
		func(i int) string {
			return fmt.Sprintf("func F%d[T any]() { F%d[A[T]](); F%d[B[T]]() }\n", i, i+1, i+1)
		},
		"func F31[T any]() {}\nfunc G() { F1[int]() }\n"), "instances"},
}

// TestLoadBombs checks that each load bomb is refused with a *LoadError at
// once, before go/types or the SSA builder runs on it. Each case runs in a
// child process with a heap watchdog (TestHugeLoadChild) and a short
// timeout, so a bomb that gets through fails the test instead of taking
// down the machine.
func TestLoadBombs(t *testing.T) {
	needSource(t)
	if testing.Short() {
		t.Skip("runs child processes")
	}
	for _, tt := range loadBombs {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestHugeLoadChild$", "-test.count=1")
			kv := hugeChildEnv + "=" + tt.src
			if len(kv) >= 128<<10 { // MAX_ARG_STRLEN of Linux, for one variable
				t.Fatalf("source of %d bytes: too long for an environment variable on Linux", len(tt.src))
			}
			cmd.Env = append(os.Environ(), kv, "GOMEMLIMIT=1GiB")
			out, err := cmd.CombinedOutput()
			if err != nil || !strings.Contains(string(out), "RESULT: ") {
				t.Fatalf("child process failed (%v):\n%.2000s", err, out)
			}
			if !strings.Contains(string(out), tt.want) {
				t.Fatalf("want a load error with %q, got:\n%.2000s", tt.want, out)
			}
		})
	}
}

// genericLib is a small library of generic helpers that call each other.
const genericLib = `type Pair[K comparable, V any] struct{ K K; V V }

func Map[T, R any](s []T, f func(T) R) []R {
	r := make([]R, 0, len(s))
	for _, x := range s {
		r = append(r, f(x))
	}
	return r
}
func Filter[T any](s []T, f func(T) bool) []T {
	var r []T
	for _, x := range s {
		if f(x) {
			r = append(r, x)
		}
	}
	return r
}
func Keys[K comparable, V any](m map[K]V) []K {
	r := make([]K, 0, len(m))
	for k := range m {
		r = append(r, k)
	}
	return r
}
func Entries[K comparable, V any](m map[K]V) []Pair[K, V] {
	return Map(Keys(m), func(k K) Pair[K, V] { return Pair[K, V]{k, m[k]} })
}
func Uniq[T comparable](s []T) []T {
	seen := map[T]bool{}
	return Filter(s, func(x T) bool { ok := !seen[x]; seen[x] = true; return ok })
}
func GroupBy[T any, K comparable](s []T, f func(T) K) map[K][]T {
	r := map[K][]T{}
	for _, x := range s {
		r[f(x)] = append(r[f(x)], x)
	}
	return r
}
func Chunk[T any](s []T, n int) [][]T {
	var r [][]T
	for len(s) > n {
		r, s = append(r, s[:n]), s[n:]
	}
	return append(r, s)
}
func Flatten[T any](s [][]T) []T {
	var r []T
	for _, x := range s {
		r = append(r, x...)
	}
	return r
}
func All[T any](s []T) []T { return Filter(s, func(T) bool { return true }) }
func Rechunk[T any](s []T, n int) []T { return Flatten(Chunk(All(s), n)) }
`

// TestLoadBombsKeepGood checks that sources close to the bombs but of
// linear size still load: the same chains behind pointers, slices and
// named generic instances used by reference, a deep but not shared struct,
// long constants, and generic code with many small instances.
func TestLoadBombsKeepGood(t *testing.T) {
	needSource(t)
	good := map[string]string{
		"genericPointers": chain("type P[A, B any] struct{ a *A; b *B }\ntype T0 int\n", 200,
			func(i int) string { return fmt.Sprintf("type T%d P[T%d, T%d]\n", i, i-1, i-1) }, "var X T200\n"),
		"structSlices": chain("type T0 int\n", 200,
			func(i int) string {
				return fmt.Sprintf("type T%d struct{ a, b []T%d; m map[string]T%d }\n", i, i-1, i-1)
			}, "var X T200\n"),
		"structDeep": chain("type T0 int\n", 300,
			func(i int) string { return fmt.Sprintf("type T%d struct{ a T%d; b, c, d int }\n", i, i-1) }, "var X T300\n"),
		"constLong": chain("const S0 = \""+strings.Repeat("x", 1000)+"\"\n", 1000,
			func(i int) string { return fmt.Sprintf("const S%d = S%d + \"y\"\n", i, i-1) }, "func F() int { return len(S1000) }\n"),
		"genericMany": chain("type Box[T any] struct{ v T }\nfunc Get[T any](b Box[T]) T { return b.v }\n", 500,
			func(i int) string { return fmt.Sprintf("type K%d int\nvar V%d = Get(Box[K%d]{})\n", i, i, i) }, ""),
		// A self-recursive function that shifts its type arguments: one round
		// of the cycle per type parameter (it was refused after one round per
		// member).
		"genericShift": "package p\n\ntype P[A any] struct{ a A }\n" +
			"func F[A, B, C, D any](n int) { if n > 0 { F[P[int], A, B, C](n - 1) } }\n" +
			"func G() { F[int, int, int, int](3) }\n",
		// Generic helpers that call each other (as samber/lo), used with 300
		// types: about 5400 instances and an instance work of 11400, 46
		// times below maxInstanceWork.
		"genericLibrary": chain(genericLib, 300, func(i int) string {
			return fmt.Sprintf("type S%d struct{ a int; b string; c map[string][]int }\n"+
				"func Use%d(m map[string]S%d, s []S%d) int {\n"+
				"\te := Entries(m)\n\tg := GroupBy(s, func(x S%d) int { return x.a })\n\tk := Uniq(Keys(m))\n"+
				"\tr := Rechunk(s, 3)\n\tp := Map(e, func(p Pair[string, S%d]) []Pair[int, []S%d] { return nil })\n"+
				"\treturn len(e) + len(g) + len(k) + len(r) + len(p)\n}\n", i, i, i, i, i, i, i)
		}, ""),
	}
	for name, src := range good {
		t.Run(name, func(t *testing.T) {
			dir, pkgPath := writePlugin(t, src)
			if _, err := Load(Options{Dir: dir, LoadTimeout: time.Minute}, pkgPath); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestLoadBombsLocalNames checks that the checks are linear in the number
// of local declarations of the same name. They looked every name up in all
// the scopes: 20000 functions with a local type T, a type that uses it and
// a constant sum took 30 s in checkSourceSize (1.7 MB of source; go/types
// checks it in a fraction of that).
func TestLoadBombsLocalNames(t *testing.T) {
	const k = 20000
	var b strings.Builder
	b.WriteString("package p\n")
	for i := 0; i < k; i++ {
		fmt.Fprintf(&b, "func f%d() { type T int; type U [1]T; var _ U; const a = \"x\"; const c = a + a; _ = c }\n", i)
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "p.go", b.String(), 0)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := checkSourceSize(fset, []*ast.File{f}, nil, nil); err != nil {
		t.Fatal(err)
	}
	// About 50 ms; the limit leaves room for a slow machine under -race.
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("checkSourceSize took %v for %d local declarations of the same names", d, k)
	}
}

// checkedPkg type-checks the single file src as checkInstances sees a
// source package.
func checkedPkg(t *testing.T, src string) (*token.FileSet, *srcPkg) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "p.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{},
		Instances: map[*ast.Ident]types.Instance{}}
	pkg, err := (&types.Config{}).Check("p", fset, []*ast.File{f}, info)
	if err != nil {
		t.Fatal(err)
	}
	return fset, &srcPkg{path: "p", files: []*ast.File{f}, info: info, types: pkg}
}

// TestInstanceCycleMeasures checks that the sites of a cycle of generic
// declarations are measured again only when the sizes of their owner grew:
// 2000 functions in one cycle, declared callees first, 25 calls each. The
// rounds over all the sites of the cycle measured 2000 x 50000 arguments
// (8.8 s and 2 GB in checkInstances).
func TestInstanceCycleMeasures(t *testing.T) {
	const n, calls = 2000, 25
	var b strings.Builder
	b.WriteString("package p\n\ntype P[A any] struct{ a A }\n")
	for i := n; i >= 1; i-- {
		fmt.Fprintf(&b, "func F%d[T any](k int) {\n\tif k > 0 {\n", i)
		for j := 0; j < calls; j++ {
			fmt.Fprintf(&b, "\t\tF%d[T](k - 1)\n", i%n+1)
		}
		b.WriteString("\t}\n}\n")
	}
	b.WriteString("func G() { F1[P[P[int]]](1) }\n")
	fset, p := checkedPkg(t, b.String())
	g, err := measureInstances(fset, []*srcPkg{p})
	if err != nil {
		t.Fatal(err)
	}
	// Each owner grows once (from 1 to the size of P[P[int]]): its sites
	// are measured at most twice.
	if max := int64(2 * n * calls); g.measures > max {
		t.Errorf("%d type arguments measured in the cycle, want at most %d", g.measures, max)
	}
}

// TestInstanceAliasOwner checks that the sites in a generic alias are
// measured with the sizes of its type parameters (typeParams returned
// nil for an alias: they counted as one node).
func TestInstanceAliasOwner(t *testing.T) {
	fset, p := checkedPkg(t, "package p\n\ntype P[X any] struct{ x X }\ntype A[T any] = P[T]\nvar V A[P[P[P[int]]]]\n")
	g, err := measureInstances(fset, []*srcPkg{p})
	if err != nil {
		t.Fatal(err)
	}
	// P[T] in A, with T = P[P[P[int]]] (4 nodes); the largest other
	// argument of P is P[P[int]] (3).
	if got := g.sizes[p.types.Scope().Lookup("P")]; len(got) != 1 || got[0] != 4 {
		t.Errorf("sizes of the parameter of P: %v, want [4]", got)
	}
}

// TestTypeSizes checks the expanded sizes of small types.
func TestTypeSizes(t *testing.T) {
	src := `package p

type N struct{ a, b int }  // 1 + 2
type M struct{ x, y N }    // value: 1 + 2*4
type R struct{ x, y *N }   // behind a pointer: 1 + 2*2
type A = [4]M              // 1 + 9
type G[T any] struct{ v, w T }
type GI = G[M]             // 1 + 1 (argument, by name) + 1 + 2*10 (the struct of G with M)
type AA = [2]struct{ a A } // one element: 1 + 1 + 11
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "p.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	s := newSizer(fset, nil)
	s.collect(f)
	s.file = f
	want := map[string][2]int64{ // size, depth
		"N": {4, 1}, "M": {10, 2}, "R": {6, 1}, "A": {11, 2}, "GI": {23, 3}, "AA": {13, 2},
	}
	for name, w := range want {
		ts := s.types[name]
		if got := s.declSize(ts); got != w[0] {
			t.Errorf("%s: size %d, want %d", name, got, w[0])
		}
		if got := s.declDepth(ts); got != w[1] {
			t.Errorf("%s: depth %d, want %d", name, got, w[1])
		}
	}
	if err := checkSourceSize(fset, []*ast.File{f}, nil, nil); err != nil {
		t.Errorf("small types refused: %v", err)
	}
	var le *LoadError
	bomb, _ := parser.ParseFile(fset, "b.go", loadBombs[2].src, 0)
	if err := checkSourceSize(fset, []*ast.File{bomb}, nil, nil); !errors.As(err, &le) {
		t.Errorf("structNamed: %v, want a *LoadError", err)
	}
}
