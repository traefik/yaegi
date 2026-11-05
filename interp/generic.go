package interp

import (
	"strconv"
	"strings"
	"sync/atomic"
)

// adot produces an AST dot(1) directed acyclic graph for the given node. For debugging only.
// func (n *node) adot() { n.astDot(dotWriter(n.interp.dotCmd), n.ident) }

// genAST returns a new AST where generic types are replaced by instantiated types.
func genAST(sc *scope, root *node, types []*itype) (*node, bool, error) {
	typeParam := map[string]*node{}
	pindex := 0
	tname := ""
	rtname := ""
	recvrPtr := false
	fixNodes := []*node{}
	var gtree func(*node, *node) (*node, error)
	sname := root.child[0].ident + "["
	if root.kind == funcDecl {
		sname = root.child[1].ident + "["
	}

	// Input type parameters must be resolved prior AST generation, as compilation
	// of generated AST may occur in a different scope.
	for _, t := range types {
		sname += t.id() + ","
	}
	sname = strings.TrimSuffix(sname, ",") + "]"

	gtree = func(n, anc *node) (*node, error) {
		nod := copyNode(n, anc, false)
		switch n.kind {
		case funcDecl, funcType:
			nod.val = nod

		case identExpr:
			// Replace generic type by instantiated one.
			nt, ok := typeParam[n.ident]
			if !ok {
				break
			}
			nod = copyNode(nt, anc, true)
			nod.typ = nt.typ

		case indexExpr:
			// Catch a possible recursive generic type definition
			if root.kind != typeSpec {
				break
			}
			if root.child[0].ident != n.child[0].ident {
				break
			}
			nod := copyNode(n.child[0], anc, false)
			fixNodes = append(fixNodes, nod)
			return nod, nil

		case fieldList:
			//  Node is the type parameters list of a generic function.
			if root.kind == funcDecl && n.anc == root.child[2] && childPos(n) == 0 {
				// Fill the types lookup table used for type substitution.
				for _, c := range n.child {
					l := len(c.child) - 1
					for _, cc := range c.child[:l] {
						if pindex >= len(types) {
							return nil, cc.cfgErrorf("undefined type for %s", cc.ident)
						}
						t, err := nodeType(c.interp, sc, c.child[l])
						if err != nil {
							return nil, err
						}
						if err := checkConstraint(types[pindex], t); err != nil {
							return nil, err
						}
						typeParam[cc.ident] = copyNode(cc, cc.anc, false)
						typeParam[cc.ident].ident = types[pindex].id()
						typeParam[cc.ident].typ = types[pindex]
						pindex++
					}
				}
				// Skip type parameters specification, so generated func doesn't look generic.
				return nod, nil
			}

			// Node is the receiver of a generic method.
			if root.kind == funcDecl && n.anc == root && childPos(n) == 0 && len(n.child) > 0 {
				rtn := n.child[0].child[1]
				// Method receiver is a generic type if it takes some type parameters.
				if rtn.kind == indexExpr || rtn.kind == indexListExpr || (rtn.kind == starExpr && (rtn.child[0].kind == indexExpr || rtn.child[0].kind == indexListExpr)) {
					if rtn.kind == starExpr {
						// Method receiver is a pointer on a generic type.
						rtn = rtn.child[0]
						recvrPtr = true
					}
					rtname = rtn.child[0].ident + "["
					for _, cc := range rtn.child[1:] {
						if pindex >= len(types) {
							return nil, cc.cfgErrorf("undefined type for %s", cc.ident)
						}
						it := types[pindex]
						typeParam[cc.ident] = copyNode(cc, cc.anc, false)
						typeParam[cc.ident].ident = it.id()
						typeParam[cc.ident].typ = it
						rtname += it.id() + ","
						pindex++
					}
					rtname = strings.TrimSuffix(rtname, ",") + "]"
				}
			}

			// Node is the type parameters list of a generic type.
			if root.kind == typeSpec && n.anc == root && childPos(n) == 1 {
				// Fill the types lookup table used for type substitution.
				tname = n.anc.child[0].ident + "["
				for _, c := range n.child {
					l := len(c.child) - 1
					for _, cc := range c.child[:l] {
						if pindex >= len(types) {
							return nil, cc.cfgErrorf("undefined type for %s", cc.ident)
						}
						it := types[pindex]
						t, err := nodeType(c.interp, sc, c.child[l])
						if err != nil {
							return nil, err
						}
						if err := checkConstraint(types[pindex], t); err != nil {
							return nil, err
						}
						typeParam[cc.ident] = copyNode(cc, cc.anc, false)
						typeParam[cc.ident].ident = it.id()
						typeParam[cc.ident].typ = it
						tname += it.id() + ","
						pindex++
					}
				}
				tname = strings.TrimSuffix(tname, ",") + "]"
				return nod, nil
			}
		}

		for _, c := range n.child {
			gn, err := gtree(c, nod)
			if err != nil {
				return nil, err
			}
			nod.child = append(nod.child, gn)
		}
		return nod, nil
	}

	// Cache instances per generic declaration, as several declarations may
	// share the same name, such as methods of different generic types.
	key := strconv.FormatInt(root.index, 10) + ":" + sname
	if nod, found := root.interp.generic[key]; found {
		return nod, true, nil
	}

	r, err := gtree(root, root.anc)
	if err != nil {
		return nil, false, err
	}
	root.interp.generic[key] = r
	r.param = append(r.param, types...)
	if tname != "" {
		for _, nod := range fixNodes {
			nod.ident = tname
		}
		r.child[0].ident = tname
	}
	if rtname != "" {
		// Replace method receiver type by synthetized ident.
		nod := r.child[0].child[0].child[1]
		if recvrPtr {
			nod = nod.child[0]
		}
		nod.kind = identExpr
		nod.ident = rtname
		nod.child = nil
	}
	// r.adot() // Used for debugging only.
	return r, false, nil
}

func copyNode(n, anc *node, recursive bool) *node {
	var i interface{}
	nindex := atomic.AddInt64(&n.interp.nindex, 1)
	nod := &node{
		debug:  n.debug,
		anc:    anc,
		interp: n.interp,
		index:  nindex,
		level:  n.level,
		nleft:  n.nleft,
		nright: n.nright,
		kind:   n.kind,
		pos:    n.pos,
		action: n.action,
		gen:    n.gen,
		val:    &i,
		rval:   n.rval,
		ident:  n.ident,
		meta:   n.meta,
	}
	nod.start = nod
	if recursive {
		for _, c := range n.child {
			nod.child = append(nod.child, copyNode(c, nod, true))
		}
	}
	return nod
}

func inferTypesFromCall(sc *scope, fun *node, args []*node) ([]*itype, error) {
	ftn := fun.typ.node

	// Type parameter names, in declaration order.
	var names []string
	for _, c := range ftn.child[0].child {
		for _, cc := range c.child[:len(c.child)-1] {
			names = append(names, cc.ident)
		}
	}
	isParam := func(name string) bool {
		for _, n := range names {
			if n == name {
				return true
			}
		}
		return false
	}

	// Bind type parameters by matching the parameter type expressions against
	// the argument types.
	bound := map[string]*itype{}
	var unify func(p *node, t *itype)
	unify = func(p *node, t *itype) {
		if t == nil {
			return
		}
		switch p.kind {
		case identExpr:
			if isParam(p.ident) && bound[p.ident] == nil {
				bound[p.ident] = t
			}
		case parenExpr, ellipsisExpr:
			unify(p.child[0], t)
		case starExpr, arrayType, chanType, chanTypeRecv, chanTypeSend:
			if t.cat == valueT && t.rtype != nil && hasElem(t.rtype) {
				unify(p.lastChild(), valueTOf(t.rtype.Elem()))
			} else if t.val != nil {
				unify(p.lastChild(), t.val)
			}
		case mapType:
			if t.cat == mapT {
				unify(p.child[0], t.key)
				unify(p.child[1], t.val)
			}
		case funcType:
			if t.cat != funcT {
				return
			}
			unifyFields := func(fl *node, types []*itype) {
				i := 0
				for _, f := range fl.child {
					l := len(f.child) - 1
					for k := 0; k < max(l, 1) && i < len(types); k++ {
						unify(f.child[l], types[i])
						i++
					}
				}
			}
			if len(p.child) > 1 {
				unifyFields(p.child[1], t.arg)
			}
			if len(p.child) > 2 {
				unifyFields(p.child[2], t.ret)
			}
		case indexExpr, indexListExpr:
			// An instance of a generic type: match its type arguments.
			for i, c := range p.child[1:] {
				if i < len(t.typeArgs) {
					unify(c, t.typeArgs[i])
				}
			}
		}
	}

	i := 0
	for _, c := range ftn.child[1].child {
		l := len(c.child) - 1
		variadic := c.child[l].kind == ellipsisExpr
		for k := 0; k < max(l, 1) && i < len(args); k++ {
			if variadic {
				// Each remaining argument is matched against the element type.
				for ; i < len(args); i++ {
					unify(c.child[l].child[0], args[i].typ)
				}
				break
			}
			unify(c.child[l], args[i].typ)
			i++
		}
	}

	types := make([]*itype, 0, len(names))
	for _, name := range names {
		t := bound[name]
		if t == nil {
			// Missing type parameters are reported when generating the instance.
			break
		}
		types = append(types, t)
	}
	return types, nil
}

func checkConstraint(it, ct *itype) error {
	if len(ct.constraint) == 0 && len(ct.ulconstraint) == 0 {
		return nil
	}
	for _, c := range ct.constraint {
		if it.equals(c) || it.matchDefault(c) {
			return nil
		}
	}
	for _, c := range ct.ulconstraint {
		if it.underlying().equals(c) || it.matchDefault(c) {
			return nil
		}
	}
	return it.node.cfgErrorf("%s does not implement %s", it.id(), ct.id())
}
