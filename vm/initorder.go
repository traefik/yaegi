package vm

import (
	"context"
	"go/token"
	"go/types"
	"sort"

	"golang.org/x/tools/go/ssa"

	"github.com/traefik/yaegi/v2/stdtypes"
)

// initOrder returns the init functions to run for the package root: the
// init functions of the interpreted packages that root imports, directly
// or not, then the one of root.
//
// The order is the one of gc. The Go spec (since Go 1.21, "Package
// initialization") sorts the packages by import path and, at each step,
// initializes the first package whose imports are all initialized. gc (the
// linker, cmd/link/internal/ld/inittask.go) does this over the graph of the
// init tasks: the compiler writes an init task only for a package that has
// init work (init functions, variables that are not static data) or that
// imports a package with an init task. A package without a task, as
// unicode/utf8, is not in the graph, and the packages that import it do not
// wait for it.
//
// The init function that go/ssa builds for a package calls the init
// functions of its imports in the order of the import declarations, so
// calling only the init of root ran the packages in the source order of the
// imports (GOROOT test fixedbugs/issue31636.go wants a, b, c for imports c,
// b, a). Each init function of go/ssa runs once (init$guard): when the
// packages are initialized here in the order of gc, the calls of an init to
// the init of its imports find them done.
//
// The graph (initGraph) has every package, interpreted or not: an
// interpreted package that imports a native or AOT package waits for its
// turn, as in gc. The packages that are not interpreted are initialized by
// the host before the program starts (and so are the packages that only they
// import).
func (m *Machine) initOrder(root *ssa.Package) []*ssa.Function {
	g := m.initGraph(root.Pkg)
	// The packages whose init runs here: the ones that the init of root
	// reaches through interpreted packages only, as the calls of the init
	// functions of go/ssa do (the init of a package that is not
	// interpreted is not called by the VM: compile.go). A package that is
	// only imported by a native or AOT package belongs to the host.
	runs := map[*types.Package]bool{}
	var reach func(p *types.Package)
	reach = func(p *types.Package) {
		if runs[p] {
			return
		}
		runs[p] = true
		for _, q := range p.Imports() {
			if f := m.initOf(q); f != nil {
				reach(q)
			}
		}
	}
	reach(root.Pkg)

	var fns []*ssa.Function
	add := func(path string) {
		if p := g[path].pkg; p != nil && p != root.Pkg && runs[p] {
			if f := m.initOf(p); f != nil {
				fns = append(fns, f)
			}
		}
	}
	var tasks, static []string
	for path, n := range g {
		if n.task {
			tasks = append(tasks, path)
		} else {
			static = append(static, path)
		}
	}
	// The packages without a task first: in gc their variables are static
	// data, set before any init runs. Their init functions in go/ssa only
	// store static values (and call the init of imports without a task).
	sort.Strings(static)
	for _, path := range static {
		add(path)
	}
	sort.Strings(tasks)
	done := map[string]bool{}
	ready := func(path string) bool {
		for _, d := range g[path].deps {
			if !done[d] {
				return false
			}
		}
		return true
	}
	for len(tasks) > 0 {
		i := 0
		for i < len(tasks) && !ready(tasks[i]) {
			i++
		}
		if i == len(tasks) {
			i = 0 // an import cycle (never in a valid program): keep going
		}
		path := tasks[i]
		tasks = append(tasks[:i], tasks[i+1:]...)
		done[path] = true
		add(path)
	}
	if f := root.Func("init"); f != nil {
		fns = append(fns, f)
	}
	return fns
}

// initNode is a package of the init graph of gc (initGraph).
type initNode struct {
	pkg  *types.Package // nil for a package that the program does not load
	task bool           // the package has an init task in gc
	deps []string       // the imports that have an init task
}

// initGraph returns the init graph of gc for the packages that root imports,
// directly or not, by import path. For a standard library package, the
// graph is the one of gc, embedded by package stdtypes (cmd/stdexport reads
// it from the object files): the export data of a package only lists the
// packages its API refers to, not its imports, and the VM cannot see which
// package has init work. For another package (type-checked from source),
// the imports come from go/types and the init work from the init function
// of go/ssa (initWork). Without the data of stdtypes (a platform without
// it, with go list), the standard library packages are handled as the
// others.
func (m *Machine) initGraph(root *types.Package) map[string]*initNode {
	std, err := stdtypes.Load()
	if err != nil {
		std = nil
	}
	g := map[string]*initNode{}
	var visit func(path string, p *types.Package) *initNode
	visit = func(path string, p *types.Package) *initNode {
		if n := g[path]; n != nil {
			return n
		}
		n := &initNode{pkg: p}
		g[path] = n
		if std != nil && stdtypes.IsStd(path) && p != root {
			deps, task := std.InitTask(path)
			n.task = task
			n.deps = deps
			for _, d := range deps {
				var q *types.Package
				if sp := m.prog.ImportedPackage(d); sp != nil {
					q = sp.Pkg
				}
				visit(d, q)
			}
			return n
		}
		if p == nil {
			return n // not loaded: nothing to wait for
		}
		for _, q := range p.Imports() {
			if visit(q.Path(), q).task {
				n.deps = append(n.deps, q.Path())
			}
		}
		n.task = len(n.deps) > 0 || p == root || m.initWork(p)
		return n
	}
	visit(root.Path(), root)
	return g
}

// initWork reports whether the package p has init work in gc: init
// functions, or package variables that are not static data. It reads the
// init function of go/ssa, which also stores the static data: a store of a
// constant, a function, the address of a global or a composite literal of
// such values is static data in gc too. Any other instruction
// (a call, a map, a closure) is init work. When the init function is not
// built (a package that is not interpreted), it reports true.
//
// gc makes a little more static: a []byte of a constant string, a call
// that it inlines to a composite literal (var x = newT()). A package with
// only such data and no import with an init task gets a task here, so the
// packages that import it wait for it, and a package with a later path
// that does not import it can run before them (TODO.md, section 4).
func (m *Machine) initWork(p *types.Package) bool {
	sp := m.prog.Package(p)
	if sp == nil {
		return true
	}
	init := sp.Func("init")
	if init == nil {
		return false
	}
	if len(init.Blocks) == 0 {
		return true
	}
	var static func(v ssa.Value) bool
	static = func(v ssa.Value) bool {
		switch v := v.(type) {
		case *ssa.Const, *ssa.Global, *ssa.Alloc:
			return true
		case *ssa.Function:
			return len(v.FreeVars) == 0
		case *ssa.FieldAddr:
			return static(v.X)
		case *ssa.IndexAddr:
			_, ok := v.Index.(*ssa.Const)
			return ok && static(v.X)
		case *ssa.Slice:
			return v.Low == nil && v.High == nil && v.Max == nil && static(v.X)
		case *ssa.MakeInterface:
			return static(v.X)
		case *ssa.ChangeType:
			return static(v.X)
		case *ssa.UnOp:
			// The load of a composite literal built in place, or of a
			// variable of the package (static if its own value is).
			if g, ok := v.X.(*ssa.Global); ok {
				return v.Op == token.MUL && g.Pkg == sp
			}
			return v.Op == token.MUL && static(v.X)
		}
		return false
	}
	for _, b := range init.Blocks {
		for _, instr := range b.Instrs {
			switch instr := instr.(type) {
			case *ssa.Jump, *ssa.If, *ssa.Return, *ssa.DebugRef:
			case *ssa.Store:
				if !static(instr.Addr) || !static(instr.Val) {
					return true
				}
			case *ssa.Call:
				// The calls to the init of the imports.
				f := instr.Call.StaticCallee()
				if f == nil || f.Name() != "init" || f.Pkg == sp || f.Parent() != nil {
					return true
				}
			case ssa.Value:
				// Alloc, FieldAddr, IndexAddr, Slice, the loads of
				// init$guard and of the composite literals...
				if !static(instr) {
					return true
				}
			default:
				return true
			}
		}
	}
	return false
}

// initOf returns the init function of the package p when the VM runs it
// (an interpreted package), else nil.
func (m *Machine) initOf(p *types.Package) *ssa.Function {
	if sp := m.prog.Package(p); sp != nil {
		if f := sp.Func("init"); f != nil && m.isInterp(f) {
			return f
		}
	}
	return nil
}

// runInits calls the init functions fns in order, as one call from the host
// (one budget and one account for all of them, as for a single init).
func (m *Machine) runInits(ctx context.Context, fns []*ssa.Function) {
	if len(fns) == 0 {
		return
	}
	compiled := make([]*Func, len(fns))
	for i, f := range fns {
		compiled[i] = m.funcOf(f)
	}
	m.top(ctx, func() {
		for _, f := range compiled {
			m.call(f, nil, nil, false)
		}
	})
}
