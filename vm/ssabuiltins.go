package vm

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// deferTo registers d on the defer stack st, made by ssa:deferstack (see
// OpDeferStack): the stack of the frame at depth st.N in the context st.R.
// A defer statement in the body of a range-over-func loop runs when the
// function that holds the loop returns, not when the body (a closure)
// returns. The defers of a context are sorted by frame depth: d goes after
// the other defers of its frame and before the defers of deeper frames.
//
// st.R is often not cx: a native iterator calls the loop body in a new
// context. It can also be a context that runs on another goroutine, when
// the iterator calls yield from a goroutine. Go allows this (the runtime
// adds the defer to an atomic list). Here the append is not locked: it is
// safe when the iterator waits for its goroutine before it returns (the
// holder only reads its defers after the iterator call), but two
// goroutines that call yield at the same time are a data race.
func (cx *ctx) deferTo(st Value, d deferred) {
	to, _ := st.R.(*ctx)
	if to == nil {
		to = cx
	}
	d.depth = int(st.N)
	if len(to.defers) == cap(to.defers) {
		to.checkDefers(len(d.args))
	}
	i := len(to.defers)
	for i > 0 && to.defers[i-1].depth > d.depth {
		i--
	}
	to.defers = append(to.defers, deferred{})
	copy(to.defers[i+1:], to.defers[i:])
	to.defers[i] = d
}

// runDefersAt returns the index in b.Instrs where the defers must run, or
// -1. ssa removes RunDefers from a function without defer statements of its
// own, but the bodies of its range-over-func loops may put defers on its
// stack (c.deferStack). The defers must run before the return, and before
// the loads of the named results that feed the return: a deferred call can
// change a named result.
func (c *compiler) runDefersAt(b *ssa.BasicBlock) int {
	if !c.deferStack || len(b.Instrs) == 0 {
		return -1
	}
	ret, ok := b.Instrs[len(b.Instrs)-1].(*ssa.Return)
	if !ok {
		return -1
	}
	isResultLoad := func(ins ssa.Instruction) bool {
		u, ok := ins.(*ssa.UnOp)
		if !ok || u.Op != token.MUL {
			return false
		}
		if _, ok := u.X.(*ssa.Alloc); !ok {
			return false
		}
		for _, r := range ret.Results {
			if r == u {
				return true
			}
		}
		return false
	}
	i := len(b.Instrs) - 1
	for i > 0 && isResultLoad(b.Instrs[i-1]) {
		i--
	}
	return i
}

// isUnsafeSize reports whether ins calls unsafe.Sizeof, Alignof or Offsetof.
func isUnsafeSize(ins ssa.Instruction) (*ssa.Call, bool) {
	call, ok := ins.(*ssa.Call)
	if !ok {
		return nil, false
	}
	bi, ok := call.Call.Value.(*ssa.Builtin)
	if !ok {
		return nil, false
	}
	switch bi.Name() {
	case "Sizeof", "Alignof", "Offsetof":
		return call, true
	}
	return nil, false
}

// skipUnsafeOperands fills c.skip and c.unsafeCalls. unsafe.Sizeof,
// Alignof and Offsetof are calls in ssa when their operand depends on a
// type parameter, and ssa computes the operand before the call. Go never
// evaluates it: unsafe.Sizeof(*p) with a nil p, or unsafe.Sizeof(f()),
// neither panics nor calls f. The instructions of the operand expression
// (their position is inside the parentheses of the call) that are used only
// by the call, or by other skipped instructions, are not compiled.
func (c *compiler) skipUnsafeOperands() {
	fn := c.f.ssa
	if !importsUnsafe(fn) {
		return
	}
	var calls []*ssa.Call
	for _, b := range fn.Blocks {
		for _, ins := range b.Instrs {
			if call, ok := isUnsafeSize(ins); ok {
				calls = append(calls, call)
			}
		}
	}
	if len(calls) == 0 {
		return
	}
	c.unsafeCalls = map[token.Pos]*ast.CallExpr{}
	if syn := fn.Syntax(); syn != nil {
		ast.Inspect(syn, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok && len(call.Args) == 1 {
				c.unsafeCalls[call.Lparen] = call
			}
			return true
		})
	}
	c.skip = map[ssa.Instruction]bool{}
	for _, call := range calls {
		syn := c.unsafeCalls[call.Pos()]
		if syn == nil {
			continue // no syntax: the operand runs (it never did harm before)
		}
		lo, hi := syn.Args[0].Pos(), syn.Args[0].End()
		inside := func(p token.Pos) bool { return p >= lo && p < hi }
		// First collect the instructions of the operand, then remove
		// the ones whose result is used by code that runs.
		cand := map[ssa.Instruction]bool{}
		var collect func(v ssa.Value)
		var elems func(v ssa.Value)
		collect = func(v ssa.Value) {
			ins, ok := v.(ssa.Instruction)
			if !ok || cand[ins] || c.skip[ins] || !operandInstr(v, inside) {
				return
			}
			cand[ins] = true
			for _, op := range ins.Operands(nil) {
				collect(*op)
			}
			elems(v)
		}
		// A composite literal: ssa allocates it (Alloc, MakeMap), takes
		// the addresses of its elements (FieldAddr, IndexAddr) and
		// stores them (Store, MapUpdate). These instructions are
		// referrers of the literal, not operands of the call.
		elems = func(v ssa.Value) {
			switch v.(type) {
			case *ssa.Alloc, *ssa.FieldAddr, *ssa.IndexAddr, *ssa.MakeMap:
			default:
				return
			}
			for _, r := range *v.Referrers() {
				if cand[r] || (r.Pos() != token.NoPos && !inside(r.Pos())) {
					continue
				}
				switch r := r.(type) {
				case *ssa.FieldAddr:
					if r.X != v {
						continue
					}
				case *ssa.IndexAddr:
					if r.X != v {
						continue
					}
				case *ssa.Store:
					if r.Addr != v {
						continue
					}
				case *ssa.MapUpdate:
					if r.Map != v {
						continue
					}
				default:
					continue
				}
				cand[r] = true
				for _, op := range r.Operands(nil) {
					collect(*op)
				}
				if a, ok := r.(ssa.Value); ok {
					elems(a)
				}
			}
		}
		collect(call.Call.Args[0])
		for changed := true; changed; {
			changed = false
			for ins := range cand {
				keep := false
				switch ins := ins.(type) {
				case *ssa.Store:
					keep = !cand[ins.Addr.(ssa.Instruction)]
				case *ssa.MapUpdate:
					keep = !cand[ins.Map.(ssa.Instruction)]
				case ssa.Value:
					for _, r := range *ins.Referrers() {
						if _, ok := r.(*ssa.DebugRef); !ok && r != call && !cand[r] {
							keep = true
						}
					}
				}
				if keep {
					delete(cand, ins)
					changed = true
				}
			}
		}
		for ins := range cand {
			c.skip[ins] = true
		}
	}
}

// operandInstr reports whether v may be an instruction of an operand
// expression: its position is inside (the operand range), or it has no
// position and it cannot panic or have side effects.
func operandInstr(v ssa.Value, inside func(token.Pos) bool) bool {
	if p := v.Pos(); p != token.NoPos {
		return inside(p)
	}
	// Implicit loads and conversions have no position. A load must be
	// from a variable or from an address of the operand (an address
	// computed outside may be fused into the load).
	switch v := v.(type) {
	case *ssa.UnOp:
		if v.Op != token.MUL {
			return false
		}
		switch x := v.X.(type) {
		case *ssa.Alloc, *ssa.Global, *ssa.FreeVar:
			return true
		default:
			return inside(x.Pos())
		}
	case *ssa.ChangeType, *ssa.Convert, *ssa.MakeInterface, *ssa.ChangeInterface,
		*ssa.Field, *ssa.FieldAddr, *ssa.Extract:
		return true
	}
	return false
}

// importsUnsafe reports whether the package of fn imports unsafe. The
// instances of generic functions have no Pkg: use their object, or the
// object of the function that holds a closure.
func importsUnsafe(fn *ssa.Function) bool {
	var p *types.Package
	for f := fn; f != nil && p == nil; f = f.Parent() {
		if f.Pkg != nil {
			p = f.Pkg.Pkg
		} else if obj := f.Object(); obj != nil {
			p = obj.Pkg()
		}
	}
	if p == nil {
		return true // unknown: look at the instructions
	}
	for _, q := range p.Imports() {
		if q.Path() == "unsafe" {
			return true
		}
	}
	return false
}

// unsafeSize compiles unsafe.Sizeof, Alignof and Offsetof when they are not
// constants: their argument depends on a type parameter. Generic functions
// are instantiated by ssa, so the types are known at compile time and the
// result is a constant register, like an ssa.Const.
func (c *compiler) unsafeSize(name string, dst int32, v *ssa.Call) {
	arg := v.Call.Args[0]
	var n int64
	switch name {
	case "Sizeof":
		n = gcSizes.Sizeof(arg.Type())
	case "Alignof":
		n = gcSizes.Alignof(arg.Type())
	case "Offsetof":
		n = c.offsetof(v)
	}
	c.template[dst] = Value{N: n}
}

// offsetof computes unsafe.Offsetof(x.f). The argument is a chain of
// Field / FieldAddr instructions: the selected field f, the embedded fields
// that the selector goes through implicitly, then the steps of x itself.
// ssa gives the implicit steps the position of the selector expression
// (the start of x). The last step of x has another position (its field
// name), so the implicit steps are the steps after f with that position.
// The implicit steps of x itself come after the last step of x.
func (c *compiler) offsetof(v *ssa.Call) int64 {
	type step struct {
		st  *types.Struct
		f   int
		pos token.Pos
	}
	var chain []step
	x := v.Call.Args[0]
	if u, ok := x.(*ssa.UnOp); ok {
		x = u.X
	}
	for done := false; !done; {
		switch s := x.(type) {
		case *ssa.FieldAddr:
			st := s.X.Type().Underlying().(*types.Pointer).Elem().Underlying().(*types.Struct)
			chain = append(chain, step{st, s.Field, s.Pos()})
			x = s.X
		case *ssa.Field:
			chain = append(chain, step{s.X.Type().Underlying().(*types.Struct), s.Field, s.Pos()})
			x = s.X
		default:
			done = true
		}
	}
	if len(chain) == 0 {
		unsupported("unsafe.Offsetof of %s", v.Call.Args[0])
	}
	start := token.NoPos // position of the selector expression
	if call := c.unsafeCalls[v.Pos()]; call != nil {
		if sel, ok := ast.Unparen(call.Args[0]).(*ast.SelectorExpr); ok {
			start = sel.Pos()
		}
	}
	if start == token.NoPos && len(chain) > 1 {
		// Without the syntax, the implicit steps cannot be told apart
		// from the steps of x: never return a wrong offset.
		unsupported("unsafe.Offsetof of %s: no syntax for the selector", v.Call.Args[0])
	}
	var off int64
	for i, s := range chain {
		if i > 0 && s.pos != start {
			break // a step of x
		}
		fields := make([]*types.Var, s.st.NumFields())
		for j := range fields {
			fields[j] = s.st.Field(j)
		}
		off += gcSizes.Offsetsof(fields)[s.f]
	}
	return off
}
