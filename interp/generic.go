package interp

import (
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

	if nod, found := root.interp.generic[sname]; found {
		return nod, true, nil
	}

	r, err := gtree(root, root.anc)
	if err != nil {
		return nil, false, err
	}
	root.interp.generic[sname] = r
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
	// Fill the map of parameter types, indexed by type param ident.
	paramTypes := map[string]*itype{}
	for _, c := range ftn.child[0].child {
		typ, err := nodeType(fun.interp, sc, c.lastChild())
		if err != nil {
			return nil, err
		}
		for _, cc := range c.child[:len(c.child)-1] {
			paramTypes[cc.ident] = typ
		}
	}

	var inferTypes func(*itype, *itype) ([]*itype, error)
	inferTypes = func(param, input *itype) ([]*itype, error) {
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
			lt := []*itype{}
			for i, f := range param.field {
				nl, err := inferTypes(f.typ, input.field[i].typ)
				if err != nil {
					return nil, err
				}
				lt = append(lt, nl...)
			}
			return lt, nil

		case funcT:
			lt := []*itype{}
			for i, t := range param.arg {
				if i >= len(input.arg) {
					break
				}
				nl, err := inferTypes(t, input.arg[i])
				if err != nil {
					return nil, err
				}
				lt = append(lt, nl...)
			}
			for i, t := range param.ret {
				if i >= len(input.ret) {
					break
				}
				nl, err := inferTypes(t, input.ret[i])
				if err != nil {
					return nil, err
				}
				lt = append(lt, nl...)
			}
			return lt, nil

		case nilT:
			if paramTypes[param.name] != nil {
				return []*itype{input}, nil
			}

		case genericT:
			return []*itype{input}, nil
		}
		return nil, nil
	}

	types := []*itype{}
	for i, c := range ftn.child[1].child {
		typ, err := nodeType(fun.interp, sc, c.lastChild())
		if err != nil {
			return nil, err
		}
		lt, err := inferTypes(typ, args[i].typ)
		if err != nil {
			return nil, err
		}
		types = append(types, lt...)
	}

	return types, nil
}

func checkConstraint(it, ct *itype) error {
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
	return it.node.cfgErrorf("%s does not implement %s", it.id(), ct.id())
}
