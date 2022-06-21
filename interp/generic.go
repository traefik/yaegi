package interp

import (
	"sync/atomic"
)

// genTree returns a new AST where generic types are replaced by instantiated types.
func genTree(root *node, types []*node) *node {
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
			//  Just ignore if node is not the type parameters list of the generic function.
			if n.anc != root.child[2] || childPos(n) != 0 {
				break
			}
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
		for _, c := range n.child {
			nod.child = append(nod.child, gtree(c, nod))
		}
		return nod
	}

	r := gtree(root, root.anc)
	//r.astDot(dotWriter(root.interp.dotCmd), root.child[1].ident) // For debugging only.
	return r
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
