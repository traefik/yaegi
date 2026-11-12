package vm

// The load-time check of plugins (preflight). Functions are compiled at
// their first call, and an instruction that cannot be compiled (a native
// symbol missing from the registry, a function written in assembly, an
// unsupported construct) only panics when it runs: for an HTTP plugin,
// in the middle of a request. preflight compiles, at load time, every
// function the plugin can reach, and Load returns one error that lists all
// the problems with their positions.

import (
	"errors"
	"fmt"
	"go/constant"
	"go/token"
	"go/types"
	"sort"
	"strings"

	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/types/typeutil"
)

// problem is an instruction that could not be compiled.
type problem struct {
	pos   token.Pos
	msg   string
	block *ssa.BasicBlock // nil: the whole function
}

// maxPreflightProblems bounds the list in the load error.
const maxPreflightProblems = 20

// PreflightError is the error of a load whose check found code the VM
// cannot run.
type PreflightError struct {
	Pkg      string
	Problems []string // "file:line:col: message", in source order
}

func (e *PreflightError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %d problem(s) found at load time: the code below cannot run in the VM", e.Pkg, len(e.Problems))
	for i, p := range e.Problems {
		if i == maxPreflightProblems {
			fmt.Fprintf(&b, "\n\t... and %d more", len(e.Problems)-i)
			break
		}
		b.WriteString("\n\t")
		b.WriteString(p)
	}
	return b.String()
}

// preflight compiles every interpreted function reachable from the members
// of root (its functions, the methods of its types, and its init, which
// reaches the init of every imported package). A function is reachable
// when an instruction refers to it (a call, a function value, a closure,
// in every branch, taken or not), or when it is a method of a type that
// is converted to an interface, or of a type inside it (an interface call
// or reflect can reach it). Compiling runs no code: the result is the same code that the first
// call would compile, so it does not change what the plugin does.
//
// Only the problems of the plugin's own code (own: its module and its
// vendored modules) fail the load. The standard library code that runs in
// the VM (instances of its generic functions, Options.Src) is walked, to
// reach the plugin functions it calls, but its problems are gaps of the VM
// that the plugin author cannot fix: they still panic when they run.
func (m *Machine) preflight(root *ssa.Package, own func(*types.Package) bool) error {
	seen := map[*ssa.Function]bool{}
	var seenT typeutil.Map
	var work []*ssa.Function
	add := func(f *ssa.Function) {
		if f != nil && !seen[f] {
			seen[f] = true
			work = append(work, f)
		}
	}
	var addMethods func(t types.Type)
	// addSet adds the methods of t, and walks the types of their
	// parameters and results: reflect reaches them with
	// Method(i).Call(...)[0].Interface() or reflect.New(t.Method(i).Type.Out(0)).
	// The signatures of the native methods of the standard library are not
	// walked (the types in them are native too), except for the methods of
	// a generic type, whose type arguments can be the plugin's.
	addSet := func(t types.Type) {
		ms := m.prog.MethodSets.MethodSet(t)
		for i := 0; i < ms.Len(); i++ {
			sel := ms.At(i)
			if isGenericMethod(sel) {
				continue
			}
			add(m.prog.MethodValue(sel))
			if fn, ok := sel.Obj().(*types.Func); ok && fn.Pkg() != nil && !own(fn.Pkg()) && !m.interpPkg(fn.Pkg()) &&
				fn.Origin().Type().(*types.Signature).RecvTypeParams().Len() == 0 {
				continue
			}
			addMethods(sel.Type()) // a *types.Signature: its params and results
		}
	}
	// addMethods adds the methods of t and of *t, and those of the types
	// inside t (struct fields, elements, keys, parameters, and the
	// parameters and results of its methods): reflect can reach them from
	// a value of t (Field(i).Interface(), Elem, Index, MapIndex, Method(i))
	// or create them from its type (reflect.New(t.Field(i).Type)).
	// seenT stops the walk on recursive types.
	addMethods = func(t types.Type) {
		if seenT.At(t) != nil {
			return
		}
		seenT.Set(t, true)
		switch u := types.Unalias(t).(type) {
		case *types.Interface, *types.TypeParam:
			return
		case *types.Named:
			if u.TypeParams().Len() > 0 && u.TypeArgs().Len() == 0 {
				return // a generic type: its instances are reached where they are used
			}
			addSet(u)
			addSet(types.NewPointer(u))
			// The types inside a native type of the standard library
			// have native methods, except in an instance of a generic
			// type, whose arguments can be the plugin's.
			if pkg := u.Obj().Pkg(); pkg != nil && !own(pkg) && !m.interpPkg(pkg) && u.TypeArgs().Len() == 0 {
				return
			}
			addMethods(u.Underlying())
		case *types.Pointer:
			addSet(u)
			addMethods(u.Elem())
		case *types.Struct:
			addSet(u) // promoted methods of embedded fields
			addSet(types.NewPointer(u))
			for i := 0; i < u.NumFields(); i++ {
				addMethods(u.Field(i).Type())
			}
		case *types.Slice:
			addMethods(u.Elem())
		case *types.Array:
			addMethods(u.Elem())
		case *types.Map:
			addMethods(u.Key())
			addMethods(u.Elem())
		case *types.Chan:
			addMethods(u.Elem())
		case *types.Signature:
			for _, tup := range []*types.Tuple{u.Params(), u.Results()} {
				for i := 0; i < tup.Len(); i++ {
					addMethods(tup.At(i).Type())
				}
			}
		}
	}
	for _, mem := range root.Members {
		switch mem := mem.(type) {
		case *ssa.Function:
			add(mem)
		case *ssa.Type:
			addMethods(mem.Type())
		}
	}
	var probs []problem
	var other []string
	var ops []*ssa.Value
	for len(work) > 0 {
		f := work[len(work)-1]
		work = work[:len(work)-1]
		if f.TypeParams().Len() > 0 && len(f.TypeArgs()) == 0 {
			continue // generic body: never run, its instances are
		}
		mine := isOwnFunc(f, own)
		if len(f.Blocks) == 0 {
			if mine && f.Pkg != nil && m.interpPkg(f.Pkg.Pkg) && f.Signature.Recv() != nil && !m.stubAsm {
				// A method without body of a package run from source:
				// compiled calls only find it at run time (with
				// StubAssembly, a call panics there).
				probs = append(probs, problem{pos: f.Pos(), msg: fmt.Sprintf(
					"unsupported: method %s has no Go body (it is written in assembly), and the VM cannot run assembly%s", f, stubAsmHint)})
			}
			continue
		}
		if !m.isInterp(f) {
			continue // native: its callers check that the registry has it
		}
		if err := m.compileChecked(m.funcOf(f)); err != nil && mine {
			other = append(other, fmt.Sprintf("%s: %v (in %s)", m.prog.Fset.Position(f.Pos()), err, f))
		}
		live := liveBlocks(f)
		if mine {
			for _, p := range m.funcOf(f).problems {
				if p.block == nil || live[p.block] {
					probs = append(probs, p)
				}
			}
		}
		for _, b := range f.Blocks {
			if !live[b] {
				continue
			}
			for _, ins := range b.Instrs {
				if mi, ok := ins.(*ssa.MakeInterface); ok {
					addMethods(mi.X.Type())
				}
				ops = ins.Operands(ops[:0])
				for _, op := range ops {
					if op == nil {
						continue
					}
					if fn, ok := (*op).(*ssa.Function); ok {
						add(fn)
					}
				}
			}
		}
	}
	if len(probs) == 0 && len(other) == 0 {
		return nil
	}
	// In source order: positions of one file set grow with the files and
	// the offsets in them.
	sort.SliceStable(probs, func(i, j int) bool { return probs[i].pos < probs[j].pos })
	seenMsg := map[string]bool{}
	var list []string
	for _, p := range probs {
		s := fmt.Sprintf("%s: %s", m.prog.Fset.Position(p.pos), p.msg)
		if !seenMsg[s] {
			seenMsg[s] = true
			list = append(list, s)
		}
	}
	list = append(list, other...)
	return &PreflightError{Pkg: root.Pkg.Path(), Problems: list}
}

// compileChecked compiles fn and returns the panic of the compiler, if any
// (the problems of single instructions are in fn.problems).
func (m *Machine) compileChecked(fn *Func) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(error); ok {
				err = e
			} else {
				err = errors.New(fmt.Sprint(r))
			}
		}
	}()
	fn.ensure()
	return nil
}

// liveBlocks returns the blocks of f that can run: the blocks reachable
// from the entry (and from the recover block), where a branch on a constant
// only goes one way. Code under `if runtime.GOOS == "windows"` (go/types
// folds the condition to a constant) or `if false` never runs: its
// problems do not fail the load.
func liveBlocks(f *ssa.Function) map[*ssa.BasicBlock]bool {
	live := map[*ssa.BasicBlock]bool{}
	var visit func(b *ssa.BasicBlock)
	visit = func(b *ssa.BasicBlock) {
		if b == nil || live[b] {
			return
		}
		live[b] = true
		succs := b.Succs
		if n := len(b.Instrs); n > 0 {
			if i, ok := b.Instrs[n-1].(*ssa.If); ok && len(succs) == 2 {
				if c, ok := i.Cond.(*ssa.Const); ok && c.Value != nil {
					if constant.BoolVal(c.Value) {
						succs = succs[:1]
					} else {
						succs = succs[1:]
					}
				}
			}
		}
		for _, s := range succs {
			visit(s)
		}
	}
	if len(f.Blocks) > 0 {
		visit(f.Blocks[0])
	}
	visit(f.Recover)
	return live
}

// isOwnFunc reports whether f is code of the plugin: a function of one of
// its packages, an instance of one, or a wrapper of one of their methods.
// A synthetic function without a package (a bound method, a thunk) counts
// as the plugin's when its method is unknown.
func isOwnFunc(f *ssa.Function, own func(*types.Package) bool) bool {
	if o := f.Origin(); o != nil {
		f = o
	}
	if f.Pkg != nil {
		return own(f.Pkg.Pkg)
	}
	if obj := f.Object(); obj != nil && obj.Pkg() != nil {
		return own(obj.Pkg())
	}
	return true
}

// isGenericMethod reports whether the method of sel is a method of a generic
// type that is not instantiated (no code exists for it).
func isGenericMethod(sel *types.Selection) bool {
	recv := sel.Recv()
	if p, ok := recv.(*types.Pointer); ok {
		recv = p.Elem()
	}
	n, ok := types.Unalias(recv).(*types.Named)
	return ok && n.TypeParams().Len() > 0 && n.TypeArgs().Len() == 0
}
