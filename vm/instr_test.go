package vm

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

// compileSrc compiles the function name of the package in src and returns
// its code.
func compileSrc(t *testing.T, src, name string) []Instr {
	t.Helper()
	m, spkg := buildSrc(t, src)
	fn := m.funcOf(spkg.Func(name))
	fn.ensure()
	return fn.code
}

// buildSrc builds the package in src (package p, it may import the standard
// library) and returns a machine that interprets it.
func buildSrc(t *testing.T, src string) (*Machine, *ssa.Package) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "p.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	pkg := types.NewPackage("p", "p")
	conf := &types.Config{Importer: importer.ForCompiler(fset, "source", nil)}
	spkg, _, err := ssautil.BuildPackage(conf, fset, pkg, []*ast.File{f}, ssa.SanityCheckFunctions)
	if err != nil {
		t.Fatal(err)
	}
	return NewMachine(spkg.Prog, map[*types.Package]bool{pkg: true}), spkg
}

// TestInstrFlags checks that the flags of the index and shift instructions
// are in Instr.F, not in Instr.T: put() reads T > 1 as a tuple size, and
// idxPtr|idxUnsigned is 3.
func TestInstrFlags(t *testing.T) {
	code := compileSrc(t, `package p
func f(p *[4]int, u uint, s, x int) (int, int) { return p[u], x << s }
`, "f")
	var idx, shift bool
	for _, in := range code {
		switch in.Op {
		case OpIndex, OpIndexAddr, OpIndexLoad:
			idx = true
			if in.F != idxPtr|idxUnsigned || in.T != 0 {
				t.Errorf("%v: F=%d T=%d, want F=%d T=0", in.Op, in.F, in.T, idxPtr|idxUnsigned)
			}
		case OpShlI:
			shift = true
			if in.F != shiftSigned || in.T != 0 {
				t.Errorf("shift: F=%d T=%d, want F=%d T=0", in.F, in.T, shiftSigned)
			}
		}
	}
	if !idx || !shift {
		t.Errorf("index or shift instruction not found in %v", code)
	}
}

// TestWrapNilChkOpcode checks that the nil check of a value method wrapper
// (ssa:wrapnilchk) is one OpWrapNilChk instruction, not an OpFunc closure:
// it runs at each call of a value method through a pointer.
func TestWrapNilChkOpcode(t *testing.T) {
	m, spkg := buildSrc(t, `package p
type V int
func (V) M() {}
`)
	ptr := types.NewPointer(spkg.Pkg.Scope().Lookup("V").Type())
	sel := spkg.Prog.MethodSets.MethodSet(ptr).Lookup(spkg.Pkg, "M")
	fn := m.funcOf(spkg.Prog.MethodValue(sel))
	fn.ensure()
	found := false
	for _, in := range fn.code {
		switch in.Op {
		case OpWrapNilChk:
			found = true
		case OpFunc:
			t.Errorf("OpFunc in the wrapper %s", fn.ssa)
		}
	}
	if !found {
		t.Errorf("no OpWrapNilChk in the wrapper %s", fn.ssa)
	}
}
