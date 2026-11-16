package vm

// The bound of the generic instances that the SSA builder creates (see
// loadbomb.go). With ssa.InstantiateGenerics, the builder creates one
// function per instance of a generic function, and the instances of the
// generic types with their methods. A generic body that instantiates
// another generic with larger type arguments,
//
//	func F1[T any]() { F2[P[T, T]]() }
//
// chained over a few functions, makes it create type arguments (and names)
// of exponential size; a body that instantiates two others makes it create
// an exponential number of instances. go/types refuses the chains that never
// end (an instantiation cycle), not the finite ones.
//
// checkInstances follows these instantiations on the checked packages,
// before the build: from the instances in non-generic code, through the
// instances that each generic body makes with its own type parameters. It
// bounds the size of the type arguments (maxTypeNodes), the number of
// instances (maxInstances), and their work (maxInstanceWork): for each site
// in a generic body, the instances of the body times the size of the type
// arguments, which the builder hashes and compares at each call. A chain of
// 400 generic functions that wraps its argument once per level, with 8000
// calls in the last body, has small sizes and few instances, but took 25 s
// to build (a work of 3.2 million).
//
// The sizes are computed on the graph of the generic declarations, callers
// first (a topological order of its strongly connected components): each
// site is measured once with the final sizes of its owner, and the sites of
// a cycle until the sizes do not change. One environment per owner, so the
// memo of the sizes is shared by its sites. (Before, every site was measured
// again at each round, with a new environment each time, up to 1000 rounds:
// the check itself allocated gigabytes on that chain.)

import (
	"go/ast"
	"go/token"
	"go/types"
	"sort"

	"golang.org/x/tools/go/types/typeutil"
)

// instSite is one instance in the source: G[args] in the body of owner (nil:
// non-generic code).
type instSite struct {
	owner  types.Object // generic func, method of a generic type, or generic type
	target types.Object // the generic func or type instantiated
	args   []types.Type
	off    int // index of the type parameter of args[0] in typeParams(target)
	pos    token.Pos
}

// ownerRange is the source range of a generic declaration.
type ownerRange struct {
	pos, end token.Pos
	obj      types.Object
}

// checkInstances bounds the generic instances of the source packages (see
// the comment of this file).
func checkInstances(fset *token.FileSet, pkgs []*srcPkg) error {
	_, err := measureInstances(fset, pkgs)
	return err
}

// measureInstances is checkInstances, with the graph it measured.
func measureInstances(fset *token.FileSet, pkgs []*srcPkg) (*instGraph, error) {
	var sites []instSite
	methods := map[types.Object][]types.Object{} // generic type -> its methods
	// localArgs: a site has a type argument that uses a local type of a
	// generic declaration (exactInstances cannot count it).
	localArgs := false
	localMemo := map[types.Type]bool{}
	for _, p := range pkgs {
		var owners []ownerRange
		for _, f := range p.files {
			for _, d := range f.Decls {
				switch d := d.(type) {
				case *ast.FuncDecl:
					obj := p.info.Defs[d.Name]
					if obj == nil {
						continue
					}
					// A generic method of a generic type (Go 1.27) is both:
					// it gets the sizes of the receiver type parameters
					// from its type, and those of its own from its sites.
					tn := genericRecv(p.info, d)
					if d.Type.TypeParams != nil || tn != nil {
						owners = append(owners, ownerRange{d.Pos(), d.End(), obj})
					}
					if tn != nil {
						methods[tn] = append(methods[tn], obj)
					}
				case *ast.GenDecl:
					for _, sp := range d.Specs {
						if ts, ok := sp.(*ast.TypeSpec); ok && ts.TypeParams != nil {
							if obj := p.info.Defs[ts.Name]; obj != nil {
								owners = append(owners, ownerRange{ts.Pos(), ts.End(), obj})
							}
						}
					}
				}
			}
		}
		sort.Slice(owners, func(i, j int) bool { return owners[i].pos < owners[j].pos })
		ownerAt := func(pos token.Pos) types.Object {
			i := sort.Search(len(owners), func(i int) bool { return owners[i].end > pos })
			if i < len(owners) && owners[i].pos <= pos {
				return owners[i].obj
			}
			return nil
		}
		for id, inst := range p.info.Instances {
			target := p.info.Uses[id]
			if target == nil {
				continue
			}
			args := make([]types.Type, inst.TypeArgs.Len())
			for i := range args {
				args[i] = inst.TypeArgs.At(i)
			}
			// The arguments of a generic method are its own type
			// parameters, after those of its receiver (typeParams).
			off := 0
			if fn, ok := target.(*types.Func); ok {
				fn = fn.Origin() // b.Map[int] on a Box[string]: the declared method
				target = fn
				off = fn.Type().(*types.Signature).RecvTypeParams().Len()
			}
			sites = append(sites, instSite{ownerAt(id.Pos()), target, args, off, id.Pos()})
			if !localArgs {
				owned := func(obj *types.TypeName) bool { return ownerAt(obj.Pos()) != nil }
				for _, a := range args {
					if usesOwnedLocal(a, owned, localMemo) {
						localArgs = true
						break
					}
				}
			}
		}
	}
	// A deterministic order: the first error is always the same.
	sort.Slice(sites, func(i, j int) bool { return sites[i].pos < sites[j].pos })

	g := newInstGraph(sites, dedupSites(sites), methods)
	g.localArgs = localArgs
	site, why := g.propagateSizes()
	if why != sizesOK && g.workOver.target != nil {
		// The work by paths passed its limit first: the error it gave
		// before the exact count (exactInstances) existed.
		site, why = g.workOver, tooMuchWork
	}
	switch why {
	case tooLargeArg:
		return g, loadErrorf("vm load: %s: generic instance too large to build: a type argument has more than %d nodes once the instances are expanded",
			fset.Position(site.pos), int64(maxTypeNodes))
	case tooMuchWork:
		return g, loadErrorf("vm load: %s: the generic instances of the plugin are too large to build: their type arguments have more than %d nodes in all",
			fset.Position(site.pos), int64(maxInstanceWork))
	case tooLongCycle:
		return g, loadErrorf("vm load: %s: a cycle of generic declarations is too long to check: more than %d types measured",
			fset.Position(site.pos), int64(maxCycleVisits))
	}
	// g.total and g.work count the paths from non-generic code, not the
	// distinct instances: the generic helpers of a cache called from many
	// generic layers that all end on the same types are counted once per
	// path (otter's node.Node[K, V]: 12732 paths). When one of them passes
	// its limit, the instances are made with their type arguments
	// (exactInstances), which stops at the same limits. The sizes are
	// bounded at this point (maxTypeNodes).
	//
	// A local type of a generic function is a new type in each instance of
	// the function (x/tools go/ssa/subst.go), but go/types has only one:
	// exactInstances would count one instance where the builder makes one
	// per instance of the owner. With such a type argument, the count by
	// paths decides, as it did before exactInstances.
	if g.localArgs {
		if g.total > maxInstances {
			return g, loadErrorf("vm load: the generic code of the plugin makes more than %d instances", int64(maxInstances))
		}
		if g.workOver.target != nil {
			return g, loadErrorf("vm load: %s: the generic instances of the plugin are too large to build: their type arguments have more than %d nodes in all",
				fset.Position(g.workOver.pos), int64(maxInstanceWork))
		}
	}
	if g.total > maxInstances || g.workOver.target != nil {
		n, over := g.exactInstances(maxInstances, maxInstanceWork)
		if n > maxInstances {
			return g, loadErrorf("vm load: the generic code of the plugin makes more than %d instances", int64(maxInstances))
		}
		if over.target != nil {
			return g, loadErrorf("vm load: %s: the generic instances of the plugin are too large to build: their type arguments have more than %d nodes in all",
				fset.Position(over.pos), int64(maxInstanceWork))
		}
	}
	return g, nil
}

// dedupSites keeps the first of the sites that have the same owner, the
// same target and identical type arguments. The SSA builder makes one
// instance per target and type arguments: G[int] written at 100 places of
// non-generic code is one instance, and F[T] written twice in the body of
// G[T] is one instance per instance of G. The instances are counted on
// these sites (countInstances, exactInstances); the work counts all the
// sites, as the builder hashes the type arguments of each call. Counted
// once per site and per path, the instances of the Souin cache plugin and
// its dependencies were 537420, where the builder made 4647 function
// instances.
func dedupSites(sites []instSite) []instSite {
	type key struct {
		owner, target types.Object
		off           int
	}
	seen := map[key]*typeutil.Map{}
	out := sites[:0:0]
	for _, st := range sites {
		k := key{st.owner, st.target, st.off}
		m := seen[k]
		if m == nil {
			m = new(typeutil.Map)
			seen[k] = m
		}
		vars := make([]*types.Var, len(st.args))
		for i, a := range st.args {
			vars[i] = types.NewParam(token.NoPos, nil, "", a)
		}
		if t := types.NewTuple(vars...); m.At(t) == nil {
			m.Set(t, true)
			out = append(out, st)
		}
	}
	return out
}

// genericRecv returns the generic type of the receiver of the method d, or
// nil.
func genericRecv(info *types.Info, d *ast.FuncDecl) types.Object {
	if d.Recv == nil || len(d.Recv.List) == 0 {
		return nil
	}
	t := d.Recv.List[0].Type
	if st, ok := t.(*ast.StarExpr); ok {
		t = st.X
	}
	x, _ := indexParts(t)
	if id, ok := x.(*ast.Ident); ok {
		return info.Uses[id]
	}
	return nil
}

// typeParams returns the type parameters of a generic object. For a
// method, those of its generic receiver come first, then its own: a
// generic method of a generic type (Go 1.27) has both, and its sites must
// be measured with the sizes of all of them (with only the receiver ones,
// a chain of generic methods on G[X] that doubles its argument at each
// level was never bounded: 11 GB for 27 levels).
func typeParams(obj types.Object) []*types.TypeParam {
	var l *types.TypeParamList
	switch obj := obj.(type) {
	case *types.Func:
		return allTypeParams(obj)
	case *types.TypeName:
		switch t := obj.Type().(type) {
		case *types.Named:
			l = t.TypeParams()
		case *types.Alias: // a generic alias: type A[T any] = P[T, T]
			l = t.TypeParams()
		}
	}
	out := make([]*types.TypeParam, l.Len())
	for i := range out {
		out[i] = l.At(i)
	}
	return out
}

// instGraph is the graph between the generic objects: an edge from the
// owner of a site to its target, and from a generic type to its methods.
// Its strongly connected components are the cycles of generic declarations.
type instGraph struct {
	sites   []instSite
	uniq    []instSite // sites without the copies (dedupSites)
	methods map[types.Object][]types.Object

	nodes   []types.Object
	edges   map[types.Object][]types.Object
	comp    map[types.Object]int
	members [][]types.Object
	order   []int   // components, callers first (topological order)
	count   []int64 // instances of each member of a component
	total   int64   // instances in all

	s         *sizer
	sizes     map[types.Object][]int64 // the largest size of each type parameter
	ver       map[types.Object]int     // changes of the sizes of each object
	envs      map[types.Object]ownerEnv
	work      int64          // instances times the size of their type arguments, so far
	workOver  instSite       // the site where work passed maxInstanceWork, if any
	localArgs bool           // a site uses a local type of a generic declaration
	measures  int64          // type arguments measured in cycles, so far (for the tests)
	grown     []types.Object // the objects whose sizes the last measure raised
}

// ownerEnv is the environment of an owner for a version of its sizes.
type ownerEnv struct {
	ver int
	env *typeEnv
}

// Why propagateSizes stops.
const (
	sizesOK = iota
	tooLargeArg
	tooMuchWork
	tooLongCycle
)

// newInstGraph makes the graph of sites. uniq are the sites without the
// copies (dedupSites): they make the edges and the instances, while all the
// sites count in the work (propagateSizes), as each call costs the SSA
// builder a hash of its type arguments.
func newInstGraph(sites, uniq []instSite, methods map[types.Object][]types.Object) *instGraph {
	g := &instGraph{
		sites: sites, uniq: uniq, methods: methods,
		edges: map[types.Object][]types.Object{}, comp: map[types.Object]int{},
		s: &sizer{typed: map[typedKey]pairSize{}}, sizes: map[types.Object][]int64{},
		ver: map[types.Object]int{}, envs: map[types.Object]ownerEnv{},
	}
	seen := map[types.Object]bool{}
	add := func(o types.Object) {
		if o != nil && !seen[o] {
			seen[o] = true
			g.nodes = append(g.nodes, o)
		}
	}
	for _, st := range uniq {
		add(st.owner)
		add(st.target)
		if st.owner != nil {
			g.edges[st.owner] = append(g.edges[st.owner], st.target)
		}
	}
	// The methods in a deterministic order (the map is not).
	var gtypes []types.Object
	for t := range methods {
		gtypes = append(gtypes, t)
	}
	sort.Slice(gtypes, func(i, j int) bool { return gtypes[i].Pos() < gtypes[j].Pos() })
	for _, t := range gtypes {
		add(t)
		for _, m := range methods[t] {
			add(m)
			g.edges[t] = append(g.edges[t], m)
		}
	}
	g.components()
	g.countInstances()
	return g
}

// components computes the strongly connected components (Tarjan).
func (g *instGraph) components() {
	index := map[types.Object]int{}
	low := map[types.Object]int{}
	onStack := map[types.Object]bool{}
	var stack []types.Object
	var rev []int // components, callees first
	next, ncomp := 0, 0
	var visit func(v types.Object)
	visit = func(v types.Object) {
		index[v], low[v] = next, next
		next++
		stack = append(stack, v)
		onStack[v] = true
		for _, w := range g.edges[v] {
			if _, ok := index[w]; !ok {
				visit(w)
				low[v] = min(low[v], low[w])
			} else if onStack[w] {
				low[v] = min(low[v], index[w])
			}
		}
		if low[v] == index[v] {
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[w] = false
				g.comp[w] = ncomp
				if w == v {
					break
				}
			}
			rev = append(rev, ncomp)
			ncomp++
		}
	}
	for _, v := range g.nodes {
		if _, ok := index[v]; !ok {
			visit(v)
		}
	}
	g.members = make([][]types.Object, ncomp)
	for _, v := range g.nodes {
		g.members[g.comp[v]] = append(g.members[g.comp[v]], v)
	}
	for i := len(rev) - 1; i >= 0; i-- {
		g.order = append(g.order, rev[i])
	}
}

// countInstances bounds the number of instances: each site makes one
// instance per instance of its owner (one in non-generic code), and the
// instances of a generic type bring their methods. The sites inside a cycle
// of generic declarations make no new instance: go/types refuses the cycles
// whose type arguments grow.
func (g *instGraph) countInstances() {
	g.count = make([]int64, len(g.members))
	for _, st := range g.uniq {
		if st.owner == nil {
			c := g.comp[st.target]
			g.count[c] = addCap(g.count[c], 1)
		}
	}
	for _, c := range g.order {
		for _, v := range g.members[c] {
			for _, w := range g.edges[v] {
				if g.comp[w] != c {
					g.count[g.comp[w]] = addCap(g.count[g.comp[w]], g.count[c])
				}
			}
		}
		g.total = addCap(g.total, mulCap(g.count[c], int64(len(g.members[c]))))
	}
}

// propagateSizes computes the largest size of each type parameter: from the
// sites of non-generic code, then component by component, callers first, so
// that the sizes of an owner are final when its sites are measured. Each
// site outside a cycle is measured once. In a cycle, a worklist measures
// again the sites of the owners whose sizes grew, until none grows (go/types
// refuses the cycles that grow); the number of rounds is not bounded by the
// members (a self-recursive F[A, B, C, D] that shifts its arguments needs
// one per type parameter), but the work of each cycle is: the types it
// measures that are not in a memo yet (maxCycleVisits). A measure with a new
// env walks the whole type argument again, so the number of measures alone
// does not bound the time: a struct of 20000 fields shifted through 1000
// type parameters took 6.8 s and 3.2 GB below a bound on the measures.
// It also sums the work of the instances: for each site, the instances of
// its owner times the size of its type arguments, which the SSA builder
// hashes and compares for each instance it makes. It returns the site where
// a limit is passed, and why.
func (g *instGraph) propagateSizes() (instSite, int) {
	byOwner := map[types.Object][]int{}
	for i, st := range g.sites {
		byOwner[st.owner] = append(byOwner[st.owner], i)
	}
	for _, i := range byOwner[nil] {
		if why := g.measure(g.sites[i], nil, 1, true); why != sizesOK {
			return g.sites[i], why
		}
	}
	for _, c := range g.order {
		members := g.members[c]
		cyclic := len(members) > 1
		for _, w := range g.edges[members[0]] {
			cyclic = cyclic || w == members[0]
		}
		var first instSite // a site of the cycle, for the error
		for _, owner := range members {
			if is := byOwner[owner]; len(is) > 0 && (first.pos == 0 || g.sites[is[0]].pos < first.pos) {
				first = g.sites[is[0]]
			}
		}
		// In a cycle, the sites inside it until the sizes do not change.
		var queue []types.Object
		queued := map[types.Object]bool{}
		if cyclic {
			queue = append(queue, members...)
			for _, m := range members {
				queued[m] = true
			}
		}
		start := g.s.visits
		for len(queue) > 0 {
			owner := queue[0]
			queue = queue[1:]
			queued[owner] = false
			for _, i := range byOwner[owner] {
				st := g.sites[i]
				if g.comp[st.target] != c {
					continue
				}
				g.measures = addCap(g.measures, int64(len(st.args)))
				if why := g.measure(st, g.envOf(owner), 0, false); why != sizesOK {
					return st, why
				}
				if g.s.visits-start > maxCycleVisits {
					return first, tooLongCycle
				}
				for _, o := range g.grown {
					if g.comp[o] == c && !queued[o] {
						queued[o] = true
						queue = append(queue, o)
					}
				}
			}
		}
		// The sizes of the members are final: each site once, with its work.
		for _, owner := range members {
			env := g.envOf(owner)
			for _, i := range byOwner[owner] {
				if why := g.measure(g.sites[i], env, g.count[c], true); why != sizesOK {
					return g.sites[i], why
				}
			}
		}
	}
	return instSite{}, sizesOK
}

// envOf returns the sizes of the type parameters of owner, for typeSize. The
// env is made again only when the sizes changed: it is part of the key of
// the memo of typeSize, so the sites of an owner share their measures.
func (g *instGraph) envOf(owner types.Object) *typeEnv {
	if e, ok := g.envs[owner]; ok && e.ver == g.ver[owner] {
		return e.env
	}
	env := &typeEnv{params: map[*types.TypeParam]pairSize{}}
	tps := typeParams(owner)
	for i, n := range g.sizes[owner] {
		if i < len(tps) {
			env.params[tps[i]] = pairSize{n, n}
		}
	}
	g.envs[owner] = ownerEnv{g.ver[owner], env}
	return env
}

// measure measures the type arguments of the site st with the sizes env of
// its owner, raises the sizes of its target (g.grown: the objects whose
// sizes grew), and, when final, adds the work of the instances of its owner.
func (g *instGraph) measure(st instSite, env *typeEnv, instances int64, final bool) int {
	g.grown = g.grown[:0]
	var size int64
	for i, a := range st.args {
		n := g.s.typeSize(a, env).ref
		if n > maxTypeNodes {
			return tooLargeArg
		}
		g.raise(st.target, st.off+i, n)
		size = addCap(size, n)
	}
	if final {
		g.work = addCap(g.work, mulCap(instances, size))
		if g.work > maxInstanceWork && g.workOver.target == nil {
			// The instances are counted by paths: the exact count
			// (exactInstances) decides at the end.
			g.workOver = st
		}
	}
	return sizesOK
}

// raise records the size n for the type parameter i of obj (and of the
// methods of a generic type); the objects whose sizes grew go to g.grown.
func (g *instGraph) raise(obj types.Object, i int, n int64) {
	for _, o := range append([]types.Object{obj}, g.methods[obj]...) {
		sz := g.sizes[o]
		for len(sz) <= i {
			sz = append(sz, 1)
		}
		if n > sz[i] {
			sz[i] = n
			g.ver[o]++
			g.grown = append(g.grown, o)
		}
		g.sizes[o] = sz
	}
}
