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
		if i >= len(args) {
			return nil, fun.cfgErrorf("not enough arguments in call to %s", fun.child[1].ident)
		}
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

// compileGenericCalls instantiates generic calls in an expression from the
// leaves up. Global type analysis needs concrete result types for calls nested
// inside operators and composite literals, before the regular CFG pass reaches
// those expressions.
func (interp *Interpreter) compileGenericCalls(sc *scope, root *node, importPath, pkgName string) error {
	// Function literal bodies have their own compilation scope and are handled by
	// the regular CFG pass. They are values here, not part of global initialization.
	if root.kind == funcLit {
		return nil
	}
	for _, child := range root.child {
		if err := interp.compileGenericCalls(sc, child, importPath, pkgName); err != nil {
			return err
		}
	}

	g, generic, err := interp.compileGenericCall(sc, root, importPath, pkgName)
	if err != nil {
		return err
	}
	if generic {
		// Match the normal CFG generic-call path: subsequent type analysis and the
		// runtime call both operate on the concrete generated function.
		root.child[0] = g
	}
	return nil
}

// compileGenericCall instantiates and compiles a generic function used as a
// call expression. Global type analysis needs the concrete result types before
// it can allocate package variables, while the regular CFG pass normally does
// this work later for calls inside function bodies.
func (interp *Interpreter) compileGenericCall(sc *scope, call *node, importPath, pkgName string) (*node, bool, error) {
	if call.kind != callExpr || len(call.child) == 0 {
		return nil, false, nil
	}
	if isBuiltinCall(call, sc) {
		return nil, false, nil
	}

	callee := call.child[0]
	if callee.isType(sc) {
		// The generic pre-pass recursively visits call-shaped arguments before
		// regular CFG compilation. Mark conversions as conversions now so the
		// call argument checker does not try to inspect them as multi-result
		// function calls through an unset callee type.
		typ, err := nodeType(interp, sc, callee)
		if err != nil {
			return nil, false, err
		}
		callee.typ = typ
		call.typ = typ
		call.action = aConvert
		switch len(call.child) {
		case 1:
			return nil, false, call.cfgErrorf("missing argument in conversion to %s", typ.id())
		case 2:
			arg := call.child[1]
			if untypedNilExpr(arg) {
				// nodeType deliberately rejects an untyped nil used by itself, but
				// nil is valid as the operand of a conversion to a nilable type.
				// The regular CFG pass resolves the nil identifier before checking
				// the conversion; mirror that ordering in this generic pre-pass.
				nilSym, _, found := sc.lookup(nilIdent)
				if !found {
					return nil, false, arg.cfgErrorf("undefined: %s", nilIdent)
				}
				arg.typ = nilSym.typ
			} else {
				arg.typ, err = nodeType(interp, sc, arg)
				if err != nil {
					return nil, false, err
				}
			}
			if err = (typecheck{scope: sc}).conversion(arg, typ); err != nil {
				return nil, false, err
			}
		default:
			return nil, false, call.cfgErrorf("too many arguments in conversion to %s", typ.id())
		}
		return nil, false, nil
	}
	var (
		fun      *node
		types    []*itype
		inferred bool
		err      error
	)

	switch callee.kind {
	case indexExpr, indexListExpr:
		if len(callee.child) < 2 {
			return nil, false, nil
		}
		ft, err := nodeType(interp, sc, callee.child[0])
		if err != nil || ft == nil || !isGeneric(ft) {
			return nil, false, err
		}
		fun = ft.node.anc
		for _, c := range callee.child[1:] {
			t, err := nodeType(interp, sc, c)
			if err != nil {
				return nil, true, err
			}
			types = append(types, t)
		}

	default:
		ft, err := nodeType(interp, sc, callee)
		if err != nil || ft == nil || !isGeneric(ft) {
			return nil, false, err
		}
		fun = ft.node.anc
		inferred = true
	}

	for _, arg := range call.child[1:] {
		t, err := nodeType(interp, sc, arg)
		if err != nil {
			return nil, true, err
		}
		arg.typ = t
	}
	if inferred {
		for _, arg := range call.child[1:] {
			arg.typ = arg.typ.defaultType(arg.rval, sc)
		}
		params := (typecheck{scope: sc}).unpackParams(call.child[1:])
		minArgs := fun.typ.numIn()
		if fun.typ.isVariadic() {
			minArgs--
		}
		if len(params) < minArgs {
			return nil, true, call.cfgErrorf("not enough arguments in call to %s", callee.name())
		}
		if !fun.typ.isVariadic() && len(params) > fun.typ.numIn() {
			return nil, true, params[fun.typ.numIn()].nod.cfgErrorf("too many arguments")
		}
		if types, err = inferTypesFromCall(sc, fun, call.child[1:]); err != nil {
			return nil, true, err
		}
	}

	g, found, err := genAST(sc, fun, types)
	if err != nil {
		return nil, true, err
	}
	if !found {
		if _, err = interp.cfg(g, fun.scope, importPath, pkgName); err != nil {
			return nil, true, err
		}
		if err = genRun(g.child[3]); err != nil {
			return nil, true, err
		}
	}
	if err = (typecheck{scope: sc}).arguments(call, call.child[1:], g, call.action == aCallSlice); err != nil {
		return nil, true, err
	}
	// A package variable initializer is hidden below a varDecl when genRun
	// later walks the complete source tree. Ensure the generated function body
	// has an execution entry point before that walk skips the declaration.
	setExec(g.child[3].start)
	if len(g.typ.ret) == 1 {
		call.typ = g.typ.ret[0]
	}
	return g, true, nil
}

func untypedNilExpr(n *node) bool {
	for n != nil && n.kind == parenExpr && len(n.child) == 1 {
		n = n.child[0]
	}
	return n != nil && n.kind == identExpr && n.ident == nilIdent || n != nil && n.kind == basicLit && n.typ != nil && n.typ.cat == nilT
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
