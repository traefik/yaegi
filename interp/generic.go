package interp

import (
	"sync/atomic"
)

// genAST returns a new AST where generic types are replaced by instantiated types.
func genAST(sc *scope, root *node, types []*node) (*node, error) {
	var gtree func(*node, *node) *node
	typeParam := map[string]*node{}
	pindex := 0

	gtree = func(n, anc *node) *node {
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

		case fieldList:
			//  Node is the type parameters list of a generic function.
			if root.kind == funcDecl && n.anc == root.child[2] && childPos(n) == 0 {
				// Fill the types lookup table used for type substitution.
				for _, c := range n.child {
					for _, cc := range c.child[:len(c.child)-1] {
						typeParam[cc.ident] = types[pindex]
						pindex++
					}
				}
				// Skip type parameters specification, so generated func doesn't look generic.
				return nod
			}

			// Node is the receiver of a generic method.
			if root.kind == funcDecl && n.anc == root && childPos(n) == 0 && len(n.child) > 0 {
				rtn := n.child[0].child[1]
				if rtn.kind == indexExpr {
					it, err := nodeType(n.interp, sc, types[pindex])
					if err != nil {
						return nil
					}
					typeParam[rtn.child[1].ident] = types[pindex]
					rid := rtn.child[0].ident + "[" + it.id() + "]"
					sym, _, ok := sc.lookup(rid)
					if !ok {
						return nil
					}
					rtn.typ = sym.typ
				}
			}

			// Node is the type parameters list of a generic type.
			if root.kind == typeSpec && n.anc == root && childPos(n) == 1 {
				// Fill the types lookup table used for type substitution.
				for _, c := range n.child {
					for _, cc := range c.child[:len(c.child)-1] {
						typeParam[cc.ident] = types[pindex]
						pindex++
					}
				}
				return nod
			}
		}
		for _, c := range n.child {
			nod.child = append(nod.child, gtree(c, nod))
		}
		return nod
	}

	r := gtree(root, root.anc)
	//r.astDot(dotWriter(root.interp.dotCmd), root.child[1].ident) // Used for debugging only.
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

func inferTypesFromCall(fun *node, args []*node) ([]*node, error) {
	ftn := fun.typ.node
	// Fill the map of parameter types, indexed by type param ident.
	types := map[string]*itype{}
	for _, c := range ftn.child[0].child {
		typ, err := nodeType(fun.interp, fun.scope, c.lastChild())
		if err != nil {
			return nil, err
		}
		for _, cc := range c.child[:len(c.child)-1] {
			types[cc.ident] = typ
		}
	}

	var inferType func(*itype, *itype) *itype
	inferType = func(param, input *itype) *itype {
		switch param.cat {
		case chanT, ptrT, sliceT:
			return inferType(param.val, input.val)
		case mapT:
			// TODO
		case structT:
			// TODO
		case funcT:
			// TODO
		case genericT:
			return input
		}
		return nil
	}

	nodes := []*node{}
	for _, c := range ftn.child[1].child {
		typ, err := nodeType(fun.interp, fun.scope, c.lastChild())
		if err != nil {
			return nil, err
		}
		t := inferType(typ, args[0].typ)
		if t == nil {
			continue
		}
		nodes = append(nodes, t.node)
	}

	return nodes, nil
}
