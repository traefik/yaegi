package vm

// The exact count of the generic instances (loadinst.go). countInstances
// counts the paths from non-generic code to each generic declaration, which
// is an upper bound: a generic helper reached through many generic layers
// that all end on the same types is one instance per type arguments, not one
// per path. exactInstances makes the instances as the SSA builder does: from
// the sites of non-generic code, each instance of an owner gives, for each
// site of its body, the target with the type arguments of the site where the
// owner's type parameters are replaced by the owner's type arguments.
// Instances with identical type arguments are one.

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/types/typeutil"
)

// exactInstances returns the number of distinct instances, or a number
// above limit as soon as it passes limit. It also sums the work of the SSA
// builder as propagateSizes does, with the instances that exist: for each
// site (all of them, each call is hashed) of each instance of its owner, the
// size of its type arguments. over is the site where the work passes
// maxWork, if it does (the count stops there).
func (g *instGraph) exactInstances(limit, maxWork int64) (n int64, over instSite) {
	uniqBy, allBy := map[types.Object][]instSite{}, map[types.Object][]instSite{}
	for _, st := range g.uniq {
		if st.owner != nil {
			uniqBy[st.owner] = append(uniqBy[st.owner], st)
		}
	}
	for _, st := range g.sites {
		allBy[st.owner] = append(allBy[st.owner], st)
	}
	var work int64
	// size is the size of the type arguments of a site, after substitution.
	size := func(args []types.Type) int64 {
		var sz int64
		for _, a := range args {
			sz = addCap(sz, g.s.typeSize(a, nil).ref)
		}
		return sz
	}
	for _, st := range allBy[nil] {
		if work = addCap(work, size(st.args)); work > maxWork {
			return 0, st
		}
	}
	type inst struct {
		obj  types.Object
		off  int
		args []types.Type
	}
	hasher := typeutil.MakeHasher()
	seen := map[types.Object]*typeutil.Map{}
	var queue []inst
	add := func(obj types.Object, off int, args []types.Type) {
		m := seen[obj]
		if m == nil {
			m = new(typeutil.Map)
			m.SetHasher(hasher)
			seen[obj] = m
		}
		if t := argsTuple(args); m.At(t) == nil {
			m.Set(t, true)
			n++
			queue = append(queue, inst{obj, off, args})
		}
	}
	for _, st := range g.uniq {
		if st.owner == nil {
			add(st.target, st.off, st.args)
		}
	}
	s := &substituter{ctxt: types.NewContext()}
	for i := 0; i < len(queue) && n <= limit; i++ {
		it := queue[i]
		tps := typeParams(it.obj)
		env := map[*types.TypeParam]types.Type{}
		for j, a := range it.args {
			if k := it.off + j; k < len(tps) {
				env[tps[k]] = a
			}
		}
		// The methods of an instance of a generic type are instances too,
		// with the type arguments of the type.
		for _, m := range g.methods[it.obj] {
			if it.off == 0 && len(typeParams(m)) == len(it.args) {
				add(m, 0, it.args)
			}
		}
		s.reset(env)
		subst := func(st instSite) []types.Type {
			args := make([]types.Type, len(st.args))
			for j, a := range st.args {
				args[j] = s.typ(a)
			}
			return args
		}
		for _, st := range allBy[it.obj] {
			if work = addCap(work, size(subst(st))); work > maxWork {
				return n, st
			}
		}
		for _, st := range uniqBy[it.obj] {
			add(st.target, st.off, subst(st))
		}
	}
	return n, instSite{}
}

// argsTuple makes a tuple of the type arguments args, a key of a
// typeutil.Map: identical tuples are the same instance.
func argsTuple(args []types.Type) *types.Tuple {
	vars := make([]*types.Var, len(args))
	for i, a := range args {
		vars[i] = types.NewParam(token.NoPos, nil, "", a)
	}
	return types.NewTuple(vars...)
}

// substituter replaces type parameters by types in a type. A type it does
// not know is kept. A local named type of a generic function is one of
// them: the SSA builder makes a new one per instance of the function, so
// measureInstances does not use the exact count when a site uses one.
type substituter struct {
	ctxt *types.Context
	env  map[*types.TypeParam]types.Type
	memo map[types.Type]types.Type
}

func (s *substituter) reset(env map[*types.TypeParam]types.Type) {
	s.env = env
	s.memo = map[types.Type]types.Type{}
}

func (s *substituter) typ(t types.Type) types.Type {
	if r, ok := s.memo[t]; ok {
		return r
	}
	r := s.typ0(t)
	s.memo[t] = r
	return r
}

func (s *substituter) typ0(t types.Type) types.Type {
	switch t := t.(type) {
	case *types.TypeParam:
		if r, ok := s.env[t]; ok {
			return r
		}
	case *types.Alias:
		return s.typ(types.Unalias(t))
	case *types.Pointer:
		return types.NewPointer(s.typ(t.Elem()))
	case *types.Slice:
		return types.NewSlice(s.typ(t.Elem()))
	case *types.Array:
		return types.NewArray(s.typ(t.Elem()), t.Len())
	case *types.Map:
		return types.NewMap(s.typ(t.Key()), s.typ(t.Elem()))
	case *types.Chan:
		return types.NewChan(t.Dir(), s.typ(t.Elem()))
	case *types.Tuple:
		return s.tuple(t)
	case *types.Signature:
		return types.NewSignatureType(nil, nil, nil, s.tuple(t.Params()), s.tuple(t.Results()), t.Variadic())
	case *types.Struct:
		fields := make([]*types.Var, t.NumFields())
		tags := make([]string, t.NumFields())
		for i := range fields {
			f := t.Field(i)
			fields[i] = types.NewField(f.Pos(), f.Pkg(), f.Name(), s.typ(f.Type()), f.Embedded())
			tags[i] = t.Tag(i)
		}
		return types.NewStruct(fields, tags)
	case *types.Interface:
		methods := make([]*types.Func, t.NumExplicitMethods())
		for i := range methods {
			m := t.ExplicitMethod(i)
			sig, _ := s.typ(m.Type()).(*types.Signature)
			methods[i] = types.NewFunc(m.Pos(), m.Pkg(), m.Name(), sig)
		}
		embeds := make([]types.Type, t.NumEmbeddeds())
		for i := range embeds {
			embeds[i] = s.typ(t.EmbeddedType(i))
		}
		return types.NewInterfaceType(methods, embeds).Complete()
	case *types.Union:
		terms := make([]*types.Term, t.Len())
		for i := range terms {
			terms[i] = types.NewTerm(t.Term(i).Tilde(), s.typ(t.Term(i).Type()))
		}
		return types.NewUnion(terms)
	case *types.Named:
		targs := t.TypeArgs()
		if targs.Len() == 0 {
			return t
		}
		args := make([]types.Type, targs.Len())
		changed := false
		for i := range args {
			args[i] = s.typ(targs.At(i))
			changed = changed || args[i] != targs.At(i)
		}
		if !changed {
			return t
		}
		if r, err := types.Instantiate(s.ctxt, t.Origin(), args, false); err == nil {
			return r
		}
	}
	return t
}

func (s *substituter) tuple(t *types.Tuple) *types.Tuple {
	if t == nil {
		return nil
	}
	vars := make([]*types.Var, t.Len())
	for i := range vars {
		v := t.At(i)
		vars[i] = types.NewParam(v.Pos(), v.Pkg(), v.Name(), s.typ(v.Type()))
	}
	return types.NewTuple(vars...)
}

// usesOwnedLocal reports whether t uses a local named type for which owned
// is true (a type declared in the body of a generic declaration). memo
// keeps the result of each type: aliases can share a type many times.
func usesOwnedLocal(t types.Type, owned func(*types.TypeName) bool, memo map[types.Type]bool) bool {
	if r, ok := memo[t]; ok {
		return r
	}
	memo[t] = false // a cycle through a type that is not done yet
	r := usesOwnedLocal0(t, owned, memo)
	memo[t] = r
	return r
}

func usesOwnedLocal0(t types.Type, owned func(*types.TypeName) bool, memo map[types.Type]bool) bool {
	in := func(ts ...types.Type) bool {
		for _, x := range ts {
			if x != nil && usesOwnedLocal(x, owned, memo) {
				return true
			}
		}
		return false
	}
	tuple := func(tu *types.Tuple) bool {
		for i := 0; tu != nil && i < tu.Len(); i++ {
			if in(tu.At(i).Type()) {
				return true
			}
		}
		return false
	}
	switch t := t.(type) {
	case *types.Alias:
		return in(types.Unalias(t))
	case *types.Pointer:
		return in(t.Elem())
	case *types.Slice:
		return in(t.Elem())
	case *types.Array:
		return in(t.Elem())
	case *types.Map:
		return in(t.Key(), t.Elem())
	case *types.Chan:
		return in(t.Elem())
	case *types.Tuple:
		return tuple(t)
	case *types.Signature:
		return tuple(t.Params()) || tuple(t.Results())
	case *types.Struct:
		for i := 0; i < t.NumFields(); i++ {
			if in(t.Field(i).Type()) {
				return true
			}
		}
	case *types.Interface:
		for i := 0; i < t.NumExplicitMethods(); i++ {
			if in(t.ExplicitMethod(i).Type()) {
				return true
			}
		}
		for i := 0; i < t.NumEmbeddeds(); i++ {
			if in(t.EmbeddedType(i)) {
				return true
			}
		}
	case *types.Union:
		for i := 0; i < t.Len(); i++ {
			if in(t.Term(i).Type()) {
				return true
			}
		}
	case *types.Named:
		obj := t.Obj()
		if obj.Pkg() != nil && obj.Parent() != nil && obj.Parent() != obj.Pkg().Scope() && owned(obj) {
			return true
		}
		for i := 0; i < t.TypeArgs().Len(); i++ {
			if in(t.TypeArgs().At(i)) {
				return true
			}
		}
	}
	return false
}
