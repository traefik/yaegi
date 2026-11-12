package vm

// Names of the types declared inside functions ("local types"), as gc
// writes them in reflect type strings.
//
// gc gives each local defined type of a package a number, in source order
// (cmd/compile/internal/noder, declCollector). The number shows in link
// strings, so in the type arguments of a reflect type string:
// "main.G[main.L·4]". A local type declared inside a generic function also
// gets the type arguments of the function instance: "main.L[int]",
// "main.M[int;string]" for type M[B any] declared in F[int] and used as
// M[string].
//
// With ssa.InstantiateGenerics, ssa makes a fresh copy of each local type
// for each instance of the function (one types.Named with a new TypeName
// that has no scope). The VM sees which instance a copy belongs to when it
// compiles the instance (noteLocalTypes).

import (
	"go/token"
	"go/types"
	"sort"
	"sync"

	"golang.org/x/tools/go/ssa"
)

type localTypes struct {
	mu sync.Mutex
	// gens: number of each local defined type of a package, by position.
	gens map[*types.Package]map[token.Pos]int
	// implicit: type arguments of the function instance that owns a fresh
	// copy of a local type (key: the copy, an origin type).
	implicit map[*types.Named][]types.Type
}

func (vr *vmReflect) localTypes() *localTypes {
	if vr == nil {
		return nil
	}
	return &vr.m.locals
}

// isLocal reports whether obj is a type declared inside a function, or the
// copy made by ssa of such a type.
func isLocal(obj *types.TypeName) bool {
	p := obj.Pkg()
	return p != nil && obj.Parent() != p.Scope() && obj.Parent() != types.Universe
}

// gen returns the gc number of the local type t, or 0.
func (lt *localTypes) gen(t *types.Named) int {
	obj := t.Origin().Obj()
	if lt == nil || !isLocal(obj) {
		return 0
	}
	lt.mu.Lock()
	defer lt.mu.Unlock()
	gens, ok := lt.gens[obj.Pkg()]
	if !ok {
		gens = localGens(obj.Pkg())
		if lt.gens == nil {
			lt.gens = map[*types.Package]map[token.Pos]int{}
		}
		lt.gens[obj.Pkg()] = gens
	}
	return gens[obj.Pos()]
}

// localGens numbers the local defined types of p like gc: in source order,
// file after file, from 1.
func localGens(p *types.Package) map[token.Pos]int {
	gens := map[token.Pos]int{}
	n := 0
	for i := 0; i < p.Scope().NumChildren(); i++ { // file scopes, in file order
		var objs []*types.TypeName
		var walk func(s *types.Scope)
		walk = func(s *types.Scope) {
			for _, name := range s.Names() {
				tn, ok := s.Lookup(name).(*types.TypeName)
				if !ok || tn.IsAlias() {
					continue
				}
				if _, ok := tn.Type().(*types.TypeParam); ok {
					continue
				}
				objs = append(objs, tn)
			}
			for j := 0; j < s.NumChildren(); j++ {
				walk(s.Child(j))
			}
		}
		fs := p.Scope().Child(i)
		for j := 0; j < fs.NumChildren(); j++ {
			walk(fs.Child(j))
		}
		sort.Slice(objs, func(a, b int) bool { return objs[a].Pos() < objs[b].Pos() })
		for _, tn := range objs {
			n++
			gens[tn.Pos()] = n
		}
	}
	return gens
}

// implicitArgs returns the type arguments of the function instance that
// owns the local type t, or nil.
func (lt *localTypes) implicitArgs(t *types.Named) []types.Type {
	if lt == nil {
		return nil
	}
	lt.mu.Lock()
	defer lt.mu.Unlock()
	return lt.implicit[t.Origin()]
}

// noteLocalTypes records the copies of local types that belong to the
// function instance fn (or to a function literal inside it).
func (m *Machine) noteLocalTypes(fn *ssa.Function) {
	top := fn
	for top.Parent() != nil {
		top = top.Parent()
	}
	targs := top.TypeArgs()
	if len(targs) == 0 || top.Origin() == nil || top.Origin().Syntax() == nil {
		return
	}
	syn := top.Origin().Syntax()
	lt := &m.locals
	seen := map[types.Type]bool{}
	var walk func(t types.Type)
	walk = func(t types.Type) {
		if t == nil || seen[t] {
			return
		}
		seen[t] = true
		switch t := types.Unalias(t).(type) {
		case *types.Named:
			o := t.Origin()
			obj := o.Obj()
			if obj.Parent() == nil && obj.Pkg() != nil && syn.Pos() <= obj.Pos() && obj.Pos() < syn.End() {
				lt.mu.Lock()
				if lt.implicit == nil {
					lt.implicit = map[*types.Named][]types.Type{}
				}
				if _, ok := lt.implicit[o]; !ok {
					lt.implicit[o] = targs
				}
				lt.mu.Unlock()
				walk(o.Underlying())
			}
			if ta := t.TypeArgs(); ta != nil {
				for i := 0; i < ta.Len(); i++ {
					walk(ta.At(i))
				}
			}
		case *types.Pointer:
			walk(t.Elem())
		case *types.Slice:
			walk(t.Elem())
		case *types.Array:
			walk(t.Elem())
		case *types.Chan:
			walk(t.Elem())
		case *types.Map:
			walk(t.Key())
			walk(t.Elem())
		case *types.Struct:
			for i := 0; i < t.NumFields(); i++ {
				walk(t.Field(i).Type())
			}
		case *types.Tuple:
			for i := 0; i < t.Len(); i++ {
				walk(t.At(i).Type())
			}
		case *types.Signature:
			walk(t.Params())
			walk(t.Results())
		case *types.Interface:
			for i := 0; i < t.NumMethods(); i++ {
				walk(t.Method(i).Type())
			}
		}
	}
	for _, p := range fn.Params {
		walk(p.Type())
	}
	for _, fv := range fn.FreeVars {
		walk(fv.Type())
	}
	for _, b := range fn.Blocks {
		for _, ins := range b.Instrs {
			if v, ok := ins.(ssa.Value); ok {
				walk(v.Type())
			}
			for _, op := range ins.Operands(nil) {
				if *op != nil {
					walk((*op).Type())
				}
			}
		}
	}
}
