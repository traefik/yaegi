package interp

import (
	"fmt"
	"strconv"
)

// TODO:
// - hierarchical scopes for symbol resolution
// - universe (global) scope
// - closures
// - slices / map expressions
// - goto
// - go routines
// - channels
// - select
// - import
// - type declarations and checking
// - type assertions and conversions
// - interfaces
// - methods
// - pointers
// - diagnostics and proper error handling
// Done:
// - basic literals
// - variable definition and assignment
// - arithmetic and logical expressions
// - if / else statement, including init
// - for statement
// - variables definition (1 scope per function)
// - function definition
// - function calls
// - assignements, including to/from multi value
// - return, including multiple values
// - for range
// - arrays
// - &&, ||, break, continue
// - switch (partial)

// Type categories
type Cat int

const (
	Unset = iota
	ArrayT
	BasicT
	FuncT
	InterfaceT
	MapT
	StructT
)

var cats = [...]string{
	Unset:      "Unset",
	ArrayT:     "ArrayT",
	BasicT:     "BasicT",
	InterfaceT: "InterfaceT",
	MapT:       "MapT",
	StructT:    "StructT",
}

func (c Cat) String() string {
	if 0 <= c && c <= Cat(len(cats)) {
		return cats[c]
	}
	return "Cat(" + strconv.Itoa(int(c)) + ")"
}

// Representation of types in interpreter
type Type struct {
	name  string  // Type name, of field name if used in struct
	index int     // Index in containing struct, for access in frame
	cat   Cat     // Type category
	field []*Type // Array of fields if StructT
	basic *Type   // Pointer to existing basic type if BasicT
}

type TypeDef map[string]*Type

// Initialize Go basic types
func initTypes() TypeDef {
	var tdef TypeDef = make(map[string]*Type)
	tdef["bool"] = &Type{name: "bool", cat: BasicT}
	tdef["float64"] = &Type{name: "float64", cat: BasicT}
	tdef["int"] = &Type{name: "int", cat: BasicT}
	tdef["string"] = &Type{name: "string", cat: BasicT}
	return tdef
}

// nodeType(tdef, n) returns the name and type definition from the corresponding
// AST subtree
func nodeType(tdef TypeDef, n *Node) *Type {
	name := n.Child[0].ident
	t := Type{name: name}
	switch n.Child[1].kind {
	case Ident:
		t.cat = BasicT
	case StructType:
		t.cat = StructT
		for i, c := range n.Child[1].Child[0].Child {
			stype := nodeType(tdef, c)
			stype.index = i
			t.field = append(t.field, stype)
		}
	}
	return &t
}

// compute size of a type, in number of entries in frame
func (t *Type) size() int {
	s := 1
	for _, f := range t.field {
		s += f.size()
	}
	return s
}

// n.Cfg() generates a control flow graph (CFG) from AST (wiring successors in AST)
// and pre-compute frame sizes and indexes for all un-named (temporary) and named
// variables.
// Following this pass, the CFG is ready to run
func (e *Node) Cfg(tdef TypeDef, sdef SymDef) int {
	symIndex := make(map[string]int)
	maxIndex := 0
	var loop, loopRestart *Node

	e.Walk(func(n *Node) bool {
		// Pre-order processing
		switch n.kind {
		case For0, ForRangeStmt:
			loop, loopRestart = n, n.Child[0]
		case For1, For2, For3, For4:
			loop, loopRestart = n, n.Child[len(n.Child)-1]
		case FuncDecl:
			symIndex = make(map[string]int)
			// allocate entries for return values at start of frame
			if len(n.Child[1].Child) == 2 {
				maxIndex = len(n.Child[1].Child[1].Child)
			} else {
				maxIndex = 0
			}
		case Switch0:
			// Make sure default clause is in last position
			c := n.Child[1].Child
			if i, l := getDefault(n), len(c)-1; i >= 0 && i != l {
				c[i], c[l] = c[l], c[i]
			}
		case GenDecl:
			t := nodeType(tdef, n.Child[0])
			tdef[t.name] = t
			// Type analysis is performed recursively and no post-order processing
			// needs to be done for types, so do not dive in subtree
			return false

		case SelectorExpr:
			return false

		case BasicLit:
			switch n.val.(type) {
			case bool:
				n.typ = tdef["bool"]
			case float64:
				n.typ = tdef["float64"]
			case int:
				n.typ = tdef["int"]
			case string:
				n.typ = tdef["string"]
			}
		}
		return true
	}, func(n *Node) {
		// Post-order processing
		switch n.kind {
		case AssignStmt:
			wireChild(n)
			n.findex = n.Child[0].findex
			// Propagate type
			// TODO: Check that existing destination type matches source type
			n.Child[0].typ = n.Child[1].typ
			n.typ = n.Child[0].typ
			maxIndex += n.typ.size()
			if n.Child[1].typ != nil && n.Child[1].typ.cat == StructT {
				n.action, n.run = CompositeLit, assignCompositeLit
			}
			fmt.Println("Assign:", n.Child[1].typ)

		case IncDecStmt:
			wireChild(n)
			n.findex = n.Child[0].findex
			// TODO: Propagate type
			fmt.Println("inc child type", n.Child[0].typ)
			n.Child[0].typ = tdef["int"]
			n.typ = n.Child[0].typ

		case AssignXStmt:
			wireChild(n)
			n.findex = n.Child[0].findex
			// TODO: Propagate type
			fmt.Println("AssignX:", n)

		case BinaryExpr, IndexExpr:
			wireChild(n)
			maxIndex++
			n.findex = maxIndex

		case BlockStmt, ExprStmt, ParenExpr:
			wireChild(n)
			n.findex = n.Child[len(n.Child)-1].findex

		case Break:
			n.tnext = loop

		case CallExpr:
			wireChild(n)
			// FIXME: should reserve as many entries as nb of ret values for called function
			// node frame index should point to the first entry
			maxIndex++
			n.findex = maxIndex
			n.val = sdef[n.Child[0].ident]

		case CaseClause:
			maxIndex++
			n.findex = maxIndex
			n.tnext = n.Child[len(n.Child)-1].Start

		case CompositeLitExpr:
			wireChild(n)
			maxIndex++
			n.findex = maxIndex
			// Set and propagate type
			n.Child[0].typ = tdef[n.Child[0].ident]
			// TODO: Check that composite litteral expr matches corresponding type
			n.typ = n.Child[0].typ
			if n.typ != nil && n.typ.cat == StructT {
				//n.action, n.run = CompositeLit, compositeLit
				maxIndex += n.typ.size() // Allocate space for struct
			}
			fmt.Println("CompositeLit:", n.typ.cat)

		case Continue:
			n.tnext = loopRestart

		case Field:
			// A single child node (no ident, just type) means that the field refers
			// to a return value, and space on frame should be accordingly allocated.
			// Otherwise, just point to corresponding location in frame, resolved in
			// ident child.
			if len(n.Child) == 1 {
				maxIndex++
				n.findex = maxIndex
			} else {
				n.findex = n.Child[0].findex
			}

		case For0: // for {}
			body := n.Child[0]
			n.Start = body.Start
			body.tnext = n.Start
			loop, loopRestart = nil, nil

		case For1: // for cond {}
			cond, body := n.Child[0], n.Child[1]
			n.Start = cond.Start
			cond.tnext = body.Start
			cond.fnext = n
			body.tnext = cond.Start
			loop, loopRestart = nil, nil

		case For2: // for init; cond; {}
			init, cond, body := n.Child[0], n.Child[1], n.Child[2]
			n.Start = init.Start
			init.tnext = cond.Start
			cond.tnext = body.Start
			cond.fnext = n
			body.tnext = cond.Start
			loop, loopRestart = nil, nil

		case For3: // for ; cond; post {}
			cond, post, body := n.Child[0], n.Child[1], n.Child[2]
			n.Start = cond.Start
			cond.tnext = body.Start
			cond.fnext = n
			body.tnext = post.Start
			post.tnext = cond.Start
			loop, loopRestart = nil, nil

		case For4: // for init; cond; post {}
			init, cond, post, body := n.Child[0], n.Child[1], n.Child[2], n.Child[3]
			n.Start = init.Start
			init.tnext = cond.Start
			cond.tnext = body.Start
			cond.fnext = n
			body.tnext = post.Start
			post.tnext = cond.Start
			loop, loopRestart = nil, nil

		case ForRangeStmt:
			loop, loopRestart = nil, nil
			n.Start = n.Child[0].Start
			n.findex = n.Child[0].findex

		case FuncDecl:
			n.findex = maxIndex + 1 // Why ????

		case Ident:
			// Lookup identifier in frame symbol table. If not found
			// should check if ident can be defined (assign, param passing...)
			// or should lookup in upper scope of variables
			// For now, simply allocate a new entry in local sym table
			if n.findex = symIndex[n.ident]; n.findex == 0 {
				maxIndex++
				symIndex[n.ident] = maxIndex
				n.findex = symIndex[n.ident]
			}

		case If0: // if cond {}
			cond, tbody := n.Child[0], n.Child[1]
			n.Start = cond.Start
			cond.tnext = tbody.Start
			cond.fnext = n
			tbody.tnext = n

		case If1: // if cond {} else {}
			cond, tbody, fbody := n.Child[0], n.Child[1], n.Child[2]
			n.Start = cond.Start
			cond.tnext = tbody.Start
			cond.fnext = fbody.Start
			tbody.tnext = n
			fbody.tnext = n

		case If2: // if init; cond {}
			init, cond, tbody := n.Child[0], n.Child[1], n.Child[2]
			n.Start = init.Start
			tbody.tnext = n
			init.tnext = cond.Start
			cond.tnext = tbody.Start
			cond.fnext = n

		case If3: // if init; cond {} else {}
			init, cond, tbody, fbody := n.Child[0], n.Child[1], n.Child[2], n.Child[3]
			n.Start = init.Start
			init.tnext = cond.Start
			cond.tnext = tbody.Start
			cond.fnext = fbody.Start
			tbody.tnext = n
			fbody.tnext = n

		case LandExpr:
			n.Start = n.Child[0].Start
			n.Child[0].tnext = n.Child[1].Start
			n.Child[0].fnext = n
			n.Child[1].tnext = n
			maxIndex++
			n.findex = maxIndex

		case LorExpr:
			n.Start = n.Child[0].Start
			n.Child[0].tnext = n
			n.Child[0].fnext = n.Child[1].Start
			n.Child[1].tnext = n
			maxIndex++
			n.findex = maxIndex

		case RangeStmt:
			n.Start = n
			n.Child[3].tnext = n
			n.tnext = n.Child[3].Start
			maxIndex++
			n.findex = maxIndex

		case ReturnStmt:
			wireChild(n)
			n.tnext = nil

		case Switch0:
			n.Start = n.Child[1].Start
			// Chain case clauses
			clauses := n.Child[1].Child
			l := len(clauses)
			for i, c := range clauses[:l-1] {
				// chain to next clause
				c.tnext = c.Child[1].Start
				c.Child[1].tnext = n
				c.fnext = clauses[i+1]
			}
			// Handle last clause
			if c := clauses[l-1]; len(c.Child) > 1 {
				// No default clause
				c.tnext = c.Child[1].Start
				c.fnext = n
				c.Child[1].tnext = n
			} else {
				// Default
				c.tnext = c.Child[0].Start
				c.Child[0].tnext = n
			}
		}
	})
	return maxIndex + 1
}

// find default case clause index of a switch statement, if any
func getDefault(n *Node) int {
	for i, c := range n.Child[1].Child {
		if len(c.Child) == 1 {
			return i
		}
	}
	return -1
}

// Wire AST nodes for CFG in subtree
func wireChild(n *Node) {
	// Set start node, in subtree (propagated to ancestors by post-order processing)
	for _, child := range n.Child {
		switch child.kind {
		case ArrayType, BasicLit, Ident:
			continue
		default:
			n.Start = child.Start
		}
		break
	}

	// Chain sequential operations inside a block (next is right sibling)
	for i := 1; i < len(n.Child); i++ {
		n.Child[i-1].tnext = n.Child[i].Start
	}

	// Chain subtree next to self
	for i := len(n.Child) - 1; i >= 0; i-- {
		switch n.Child[i].kind {
		case ArrayType, BasicLit, Ident:
			continue
		case Break, Continue, ReturnStmt:
			// tnext is already computed, no change
		default:
			n.Child[i].tnext = n
		}
		break
	}
}

// optimisation: rewire CFG to skip nop nodes
func (e *Node) OptimCfg() {
	e.Walk(nil, func(n *Node) {
		for n.tnext != nil && n.tnext.action == Nop {
			n.tnext = n.tnext.tnext
		}
	})
}
