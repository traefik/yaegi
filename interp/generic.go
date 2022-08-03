package interp

import (
	"strings"
	"sync/atomic"
)

// genAST returns a new AST where generic types are replaced by instantiated types.
func genAST(sc *scope, root *node, types []*node) (*node, error) {
	typeParam := map[string]*node{}
	pindex := 0
	tname := ""
	rtname := ""
	recvrPtr := false
	fixNodes := []*node{}
	var gtree func(*node, *node) (*node, error)

	gtree = func(n, anc *node) (*node, error) {
		nod := copyNode(n, anc)
		switch n.kind {
		case funcDecl, funcType:
			nod.val = nod

		case identExpr:
			// Replace generic type by instantiated one.
			nt, ok := typeParam[n.ident]
			if !ok {
				break
			}
			nod = copyNode(nt, anc)

		case indexExpr:
			// Catch a possible recursive generic type definition
			if root.kind != typeSpec {
				break
			}
			if root.child[0].ident != n.child[0].ident {
				break
			}
			nod := copyNode(n.child[0], anc)
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
						if err := checkConstraint(sc, types[pindex], c.child[l]); err != nil {
							return nil, err
						}
						typeParam[cc.ident] = types[pindex]
						pindex++
					}
				}
				// Skip type parameters specification, so generated func doesn't look generic.
				return nod, nil
			}

			// Node is the receiver of a generic method.
			if root.kind == funcDecl && n.anc == root && childPos(n) == 0 && len(n.child) > 0 {
				rtn := n.child[0].child[1]
				if rtn.kind == indexExpr || (rtn.kind == starExpr && rtn.child[0].kind == indexExpr) {
					// Method receiver is a generic type.
					if rtn.kind == starExpr && rtn.child[0].kind == indexExpr {
						// Method receiver is a pointer on a generic type.
						rtn = rtn.child[0]
						recvrPtr = true
					}
					rtname = rtn.child[0].ident + "["
					for _, cc := range rtn.child[1:] {
						if pindex >= len(types) {
							return nil, cc.cfgErrorf("undefined type for %s", cc.ident)
						}
						it, err := nodeType(n.interp, sc, types[pindex])
						if err != nil {
							return nil, err
						}
						typeParam[cc.ident] = types[pindex]
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
						it, err := nodeType(n.interp, sc, types[pindex])
						if err != nil {
							return nil, err
						}
						if err := checkConstraint(sc, types[pindex], c.child[l]); err != nil {
							return nil, err
						}
						typeParam[cc.ident] = types[pindex]
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

	r, err := gtree(root, root.anc)
	if err != nil {
		return nil, err
	}
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
	// r.astDot(dotWriter(root.interp.dotCmd), root.child[1].ident) // Used for debugging only.
	return r, nil
}

func copyNode(n, anc *node) *node {
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
	return nod
}

func inferTypesFromCall(sc *scope, fun *node, args []*node) ([]*node, error) {
	ftn := fun.typ.node
	// Fill the map of parameter types, indexed by type param ident.
	types := map[string]*itype{}
	for _, c := range ftn.child[0].child {
		typ, err := nodeType(fun.interp, sc, c.lastChild())
		if err != nil {
			return nil, err
		}
		for _, cc := range c.child[:len(c.child)-1] {
			types[cc.ident] = typ
		}
	}

	var inferTypes func(*itype, *itype) ([]*node, error)
	inferTypes = func(param, input *itype) ([]*node, error) {
		switch param.cat {
		case chanT, ptrT, sliceT:
			return inferTypes(param.val, input.val)

		case mapT:
			k, err := inferTypes(param.key, input.key)
			if err != nil {
				return nil, err
			}
			v, err := inferTypes(param.val, input.val)
			if err != nil {
				return nil, err
			}
			return append(k, v...), nil

		case structT:
			nods := []*node{}
			for i, f := range param.field {
				nl, err := inferTypes(f.typ, input.field[i].typ)
				if err != nil {
					return nil, err
				}
				nods = append(nods, nl...)
			}
			return nods, nil

		case funcT:
			nods := []*node{}
			for i, t := range param.arg {
				nl, err := inferTypes(t, input.arg[i])
				if err != nil {
					return nil, err
				}
				nods = append(nods, nl...)
			}
			for i, t := range param.ret {
				nl, err := inferTypes(t, input.ret[i])
				if err != nil {
					return nil, err
				}
				nods = append(nods, nl...)
			}
			return nods, nil

		case genericT:
			return []*node{input.node}, nil
		}
		return nil, nil
	}

	nodes := []*node{}
	for i, c := range ftn.child[1].child {
		typ, err := nodeType(fun.interp, sc, c.lastChild())
		if err != nil {
			return nil, err
		}
		nods, err := inferTypes(typ, args[i].typ)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, nods...)
	}

	return nodes, nil
}

func checkConstraint(sc *scope, input, constraint *node) error {
	ct, err := nodeType(constraint.interp, sc, constraint)
	if err != nil {
		return err
	}
	it, err := nodeType(input.interp, sc, input)
	if err != nil {
		return err
	}
	if len(ct.constraint) == 0 && len(ct.ulconstraint) == 0 {
		return nil
	}
	for _, c := range ct.constraint {
		if it.equals(c) {
			return nil
		}
	}
	for _, c := range ct.ulconstraint {
		if it.underlying().equals(c) {
			return nil
		}
	}
	return input.cfgErrorf("%s does not implement %s", input.typ.id(), ct.id())
}
