package vm

// The //go:nointerface directive of gc: with GOEXPERIMENT=fieldtrack, a
// method marked with it is not in the run-time method set of its type. It
// does not count for interface satisfaction (type assertions, type switches,
// reflect Implements, the interfaces native code checks, such as
// fmt.Stringer), and reflect does not list it (NumMethod, Method,
// MethodByName). This is also true when the method is promoted from an
// embedded field or declared on a generic type. Direct calls still work.
//
// Every check of the VM goes through methodSet or typesImplements below, so
// the VM always agrees with itself.

import (
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"go/types"
	"os/exec"
	"slices"
	"strings"
	"sync"

	"golang.org/x/tools/go/packages"
)

// methodSet returns the run-time method set of t: the go/types method set,
// without the methods marked //go:nointerface when m.FieldTrack is set.
func (m *Machine) methodSet(t types.Type) []*types.Selection {
	ms := m.prog.MethodSets.MethodSet(t)
	out := make([]*types.Selection, 0, ms.Len())
	for i := 0; i < ms.Len(); i++ {
		sel := ms.At(i)
		if fn, ok := sel.Obj().(*types.Func); ok && m.isNoInterface(fn) {
			continue
		}
		out = append(out, sel)
	}
	return out
}

// typesImplements is types.Implements for the run-time method set of t (see
// methodSet).
func (m *Machine) typesImplements(t types.Type, it *types.Interface) bool {
	return types.Implements(t, it) && !m.hasNoInterface(t, it)
}

// hasNoInterface reports whether t gets one of the methods of it from a
// method marked //go:nointerface (only when m.FieldTrack is set).
func (m *Machine) hasNoInterface(t types.Type, it *types.Interface) bool {
	if !m.FieldTrack {
		return false
	}
	for i := 0; i < it.NumMethods(); i++ {
		im := it.Method(i)
		obj, _, _ := types.LookupFieldOrMethod(t, false, im.Pkg(), im.Name())
		if fn, ok := obj.(*types.Func); ok && m.isNoInterface(fn) {
			return true
		}
	}
	return false
}

// isNoInterface reports whether m.FieldTrack is set and the declaration of
// the method fn has the //go:nointerface directive.
func (m *Machine) isNoInterface(fn *types.Func) bool {
	if !m.FieldTrack {
		return false
	}
	f := m.prog.FuncValue(fn.Origin())
	if f == nil {
		return false
	}
	decl, ok := f.Syntax().(*ast.FuncDecl)
	if !ok || decl.Recv == nil {
		return false
	}
	return m.noIface.marked(m.prog.Fset, decl)
}

// noIfaceFiles finds the //go:nointerface directives of source files.
type noIfaceFiles struct {
	mu    sync.Mutex
	files map[string]map[int]bool // file name -> offsets of the marked funcs
}

// marked reports whether a //go:nointerface directive applies to decl.
// As in gc, the directive applies to the next declaration: it is any comment
// between the end of the previous top-level declaration and decl, also when
// a blank line separates it from decl (so not only decl.Doc).
func (n *noIfaceFiles) marked(fset *token.FileSet, decl *ast.FuncDecl) bool {
	if decl.Doc != nil && slices.ContainsFunc(decl.Doc.List, isNoIfaceComment) {
		return true
	}
	tf := fset.File(decl.Pos())
	if tf == nil {
		return false
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	offs, ok := n.files[tf.Name()]
	if !ok {
		offs = noIfaceOffsets(tf.Name())
		if n.files == nil {
			n.files = map[string]map[int]bool{}
		}
		n.files[tf.Name()] = offs
	}
	return offs[tf.Offset(decl.Pos())]
}

// noIfaceOffsets parses the file name and returns the offsets of the
// functions a //go:nointerface directive applies to. A file that cannot be
// read gives none (decl.Doc is still checked).
func noIfaceOffsets(name string) map[int]bool {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, name, nil, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil
	}
	offs := map[int]bool{}
	prev := f.Name.End()
	ci := 0
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok {
			for ci < len(f.Comments) && f.Comments[ci].End() <= fd.Pos() {
				cg := f.Comments[ci]
				if cg.Pos() >= prev && slices.ContainsFunc(cg.List, isNoIfaceComment) {
					offs[fset.Position(fd.Pos()).Offset] = true
				}
				ci++
			}
		}
		prev = d.End()
	}
	return offs
}

// isNoIfaceComment reports whether c is the //go:nointerface directive (gc
// ignores what comes after a space).
func isNoIfaceComment(c *ast.Comment) bool {
	text, ok := strings.CutPrefix(c.Text, "//")
	if !ok {
		return false
	}
	verb, _, _ := strings.Cut(text, " ")
	return verb == "go:nointerface"
}

// fieldTrackOn reports whether the GOEXPERIMENT value s enables fieldtrack
// (later entries win, "none" turns every experiment off).
func fieldTrackOn(s string) bool {
	on := false
	for _, e := range strings.Split(s, ",") {
		switch strings.TrimSpace(e) {
		case "fieldtrack":
			on = true
		case "nofieldtrack", "none":
			on = false
		}
	}
	return on
}

// goListFieldTrack reports whether the go command, run in dir, builds pkgs
// with GOEXPERIMENT=fieldtrack: the experiments that set the build tags of
// go list (environment, `go env -w`). To keep the load fast, it runs
// `go env` only when the source of pkgs has a //go:nointerface directive.
func goListFieldTrack(dir string, pkgs []*packages.Package) bool {
	found := false
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		for _, f := range p.Syntax {
			for _, cg := range f.Comments {
				if slices.ContainsFunc(cg.List, isNoIfaceComment) {
					found = true
				}
			}
		}
	})
	if !found {
		return false
	}
	cmd := exec.Command("go", "env", "GOEXPERIMENT")
	cmd.Dir = dir
	out, err := cmd.Output()
	return err == nil && fieldTrackOn(strings.TrimSpace(string(out)))
}

// buildFieldTrack reports whether the build tags of go/build (used by the
// toolchain-free plugin loader) include goexperiment.fieldtrack.
func buildFieldTrack() bool {
	return slices.Contains(build.Default.ToolTags, "goexperiment.fieldtrack")
}
