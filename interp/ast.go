package interp

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"strconv"
)

// Kind defines the kind of AST, i.e. the grammar category
type Kind uint

// Node kinds for the go language
const (
	Undef Kind = iota
	Address
	ArrayType
	AssignStmt
	AssignXStmt
	BasicLit
	BinaryExpr
	BlockStmt
	BranchStmt
	Break
	CallExpr
	CaseBody
	CaseClause
	ChanType
	CommClause
	CompositeLitExpr
	ConstDecl
	Continue
	DeclStmt
	DeferStmt
	Define
	DefineX
	Ellipsis
	ExprStmt
	Fallthrough
	Field
	FieldList
	File
	For0         // for {}
	For1         // for cond {}
	For2         // for init; cond; {}
	For3         // for ; cond; post {}
	For3a        // for init; ; post {}
	For4         // for init; cond; post {}
	ForRangeStmt // for range
	FuncDecl
	FuncLit
	FuncType
	Go
	GoStmt
	Goto
	Ident
	If0 // if cond {}
	If1 // if cond {} else {}
	If2 // if init; cond {}
	If3 // if init; cond {} else {}
	ImportDecl
	ImportSpec
	IncDecStmt
	IndexExpr
	InterfaceType
	LandExpr
	LorExpr
	KeyValueExpr
	MapType
	ParenExpr
	RangeStmt
	ReturnStmt
	Rvalue
	Rtype
	SelectStmt
	SelectorExpr
	SelectorImport
	SelectorSrc
	SendStmt
	SliceExpr
	StarExpr
	StructType
	Switch
	TypeAssertExpr
	TypeDecl
	TypeSpec
	UnaryExpr
	ValueSpec
	VarDecl
)

var kinds = [...]string{
	Undef:            "Undef",
	Address:          "Address",
	ArrayType:        "ArrayType",
	AssignStmt:       "AssignStmt",
	AssignXStmt:      "AssignXStmt",
	BasicLit:         "BasicLit",
	BinaryExpr:       "BinaryExpr",
	BlockStmt:        "BlockStmt",
	BranchStmt:       "BranchStmt",
	Break:            "Break",
	CallExpr:         "CallExpr",
	CaseBody:         "CaseBody",
	CaseClause:       "CaseClause",
	ChanType:         "ChanType",
	CommClause:       "CommClause",
	CompositeLitExpr: "CompositeLitExpr",
	ConstDecl:        "ConstDecl",
	Continue:         "Continue",
	DeclStmt:         "DeclStmt",
	DeferStmt:        "DeferStmt",
	Define:           "Define",
	DefineX:          "DefineX",
	Ellipsis:         "Ellipsis",
	ExprStmt:         "ExprStmt",
	Fallthrough:      "Fallthrough",
	Field:            "Field",
	FieldList:        "FieldList",
	File:             "File",
	For0:             "For0",
	For1:             "For1",
	For2:             "For2",
	For3:             "For3",
	For3a:            "For3a",
	For4:             "For4",
	ForRangeStmt:     "ForRangeStmt",
	FuncDecl:         "FuncDecl",
	FuncType:         "FuncType",
	FuncLit:          "FuncLit",
	Go:               "Go",
	GoStmt:           "GoStmt",
	Goto:             "Goto",
	Ident:            "Ident",
	If0:              "If0",
	If1:              "If1",
	If2:              "If2",
	If3:              "If3",
	ImportDecl:       "ImportDecl",
	ImportSpec:       "ImportSpec",
	IncDecStmt:       "IncDecStmt",
	IndexExpr:        "IndexExpr",
	InterfaceType:    "InterfaceType",
	KeyValueExpr:     "KeyValueExpr",
	LandExpr:         "LandExpr",
	LorExpr:          "LorExpr",
	MapType:          "MapType",
	ParenExpr:        "ParenExpr",
	RangeStmt:        "RangeStmt",
	ReturnStmt:       "ReturnStmt",
	Rvalue:           "Rvalue",
	Rtype:            "Rtype",
	SelectStmt:       "SelectStmt",
	SelectorExpr:     "SelectorExpr",
	SelectorImport:   "SelectorImport",
	SelectorSrc:      "SelectorSrc",
	SendStmt:         "SendStmt",
	SliceExpr:        "SliceExpr",
	StarExpr:         "StarExpr",
	StructType:       "StructType",
	Switch:           "Switch",
	TypeAssertExpr:   "TypeAssertExpr",
	TypeDecl:         "TypeDecl",
	TypeSpec:         "TypeSpec",
	UnaryExpr:        "UnaryExpr",
	ValueSpec:        "ValueSpec",
	VarDecl:          "VarDecl",
}

func (k Kind) String() string {
	if k < Kind(len(kinds)) {
		return kinds[k]
	}
	return "Kind(" + strconv.Itoa(int(k)) + ")"
}

// AstError represents an error during AST build stage
type AstError error

// Action defines the node action to perform at execution
type Action uint

// Node actions for the go language
const (
	Nop Action = iota
	Addr
	Assign
	AssignX
	Add
	AddAssign
	And
	AndAssign
	AndNot
	AndNotAssign
	Call
	Case
	CompositeLit
	Dec
	Defer
	Equal
	Greater
	GreaterEqual
	GetFunc
	GetIndex
	Inc
	Land
	Lor
	Lower
	LowerEqual
	Mul
	MulAssign
	Negate
	Not
	NotEqual
	Or
	OrAssign
	Quo
	QuoAssign
	Range
	Recv
	Rem
	RemAssign
	Return
	Select
	Send
	Shl
	ShlAssign
	Shr
	ShrAssign
	Slice
	Slice0
	Star
	Sub
	SubAssign
	TypeAssert
	Xor
	XorAssign
)

var actions = [...]string{
	Nop:          "nop",
	Addr:         "&",
	Assign:       "=",
	AssignX:      "X=",
	Add:          "+",
	AddAssign:    "+=",
	And:          "&",
	AndAssign:    "&=",
	AndNot:       "&^",
	AndNotAssign: "&^=",
	Call:         "call",
	Case:         "case",
	CompositeLit: "compositeLit",
	Dec:          "--",
	Defer:        "defer",
	Equal:        "==",
	Greater:      ">",
	GetFunc:      "getFunc",
	GetIndex:     "getIndex",
	Inc:          "++",
	Land:         "&&",
	Lor:          "||",
	Lower:        "<",
	Mul:          "*",
	MulAssign:    "*=",
	Negate:       "-",
	Not:          "!",
	NotEqual:     "!=",
	Quo:          "/",
	QuoAssign:    "/=",
	Range:        "range",
	Recv:         "<-",
	Rem:          "%",
	RemAssign:    "%=",
	Return:       "return",
	Send:         "<~",
	Shl:          "<<",
	ShlAssign:    "<<=",
	Shr:          ">>",
	ShrAssign:    ">>=",
	Slice:        "slice",
	Slice0:       "slice0",
	Star:         "*",
	Sub:          "-",
	SubAssign:    "-=",
	TypeAssert:   "TypeAssert",
	Xor:          "^",
	XorAssign:    "^=",
}

func (a Action) String() string {
	if a < Action(len(actions)) {
		return actions[a]
	}
	return "Action(" + strconv.Itoa(int(a)) + ")"
}

func firstToken(src string) token.Token {
	var s scanner.Scanner
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(src))
	s.Init(file, []byte(src), nil, 0)

	_, tok, _ := s.Scan()
	return tok
}

// Note: no type analysis is performed at this stage, it is done in pre-order processing
// of CFG, in order to accommodate forward type declarations

// ast parses src string containing Go code and generates the corresponding AST.
// The package name and the AST root node are returned.
func (interp *Interpreter) ast(src, name string) (string, *Node, error) {
	var inFunc bool

	// Allow incremental parsing of declarations or statements, by inserting them in a pseudo
	// file package or function.
	// Those statements or declarations will be always evaluated in the global scope
	switch firstToken(src) {
	case token.PACKAGE:
		// nothing to do
	case token.CONST, token.FUNC, token.IMPORT, token.TYPE, token.VAR:
		src = "package _;" + src
	default:
		inFunc = true
		src = "package _; func _() {" + src + "}"
	}

	fset := token.NewFileSet() // positions are relative to fset
	f, err := parser.ParseFile(fset, name, src, 0)
	if err != nil {
		return "", nil, err
	}

	var root *Node
	var anc astNode
	var st nodestack
	var nbAssign int // number of assign operations in  the current assign statement
	var typeSpec bool
	var pkgName string

	addChild := func(root **Node, anc astNode, pos token.Pos, kind Kind, action Action) *Node {
		interp.nindex++
		var i interface{}
		n := &Node{anc: anc.node, interp: interp, index: interp.nindex, fset: fset, pos: pos, kind: kind, action: action, val: &i, gen: builtin[action]}
		n.start = n
		if anc.node == nil {
			*root = n
		} else {
			anc.node.child = append(anc.node.child, n)
			if anc.node.action == Assign {
				if nbAssign > 1 {
					if !typeSpec && len(anc.node.child) == 2*nbAssign {
						// All LHS and RSH assign child are now defined, so split multiple assign
						// statement into single assign statements.
						newAnc := anc.node.anc
						var newChild []*Node
						for i := 0; i < nbAssign; i++ {
							// set new single assign
							interp.nindex++
							na := &Node{anc: anc.node.anc, interp: interp, index: interp.nindex, pos: pos, kind: anc.node.kind, action: anc.node.action, val: new(interface{}), gen: anc.node.gen}
							na.start = na
							newChild = append(newChild, na)
							// Set single assign left hand side
							anc.node.child[i].anc = na
							na.child = append(na.child, anc.node.child[i])
							// Set single assign right hand side
							anc.node.child[i+nbAssign].anc = na
							na.child = append(na.child, anc.node.child[i+nbAssign])
						}
						newAnc.child = newChild
					} else if typeSpec && len(anc.node.child) == 2*nbAssign+1 {
						// All LHS and RHS assign child are now defined, so split multiple assign
						// statement into single assign statements. Set type for each assignment.
						typeSpec = false
						newAnc := anc.node.anc
						var newChild []*Node
						typeNode := anc.node.child[nbAssign]
						for i := 0; i < nbAssign; i++ {
							// set new single assign
							interp.nindex++
							na := &Node{anc: anc.node.anc, interp: interp, index: interp.nindex, pos: pos, kind: anc.node.kind, action: anc.node.action, val: new(interface{}), gen: anc.node.gen}
							na.start = na
							newChild = append(newChild, na)
							// set new type for this assignment
							interp.nindex++
							nt := &Node{anc: na, interp: interp, ident: typeNode.ident, index: interp.nindex, pos: pos, kind: typeNode.kind, action: typeNode.action, val: new(interface{}), gen: typeNode.gen}
							// Set single assign left hand side
							anc.node.child[i].anc = na
							na.child = append(na.child, anc.node.child[i])
							// Set assignment type
							na.child = append(na.child, nt)
							// Set single assign right hand side
							anc.node.child[i+nbAssign+1].anc = na
							na.child = append(na.child, anc.node.child[i+nbAssign+1])
						}
						newAnc.child = newChild
					}
				}
			} else if anc.node.action == Case {
				ancAst := anc.ast.(*ast.CaseClause)
				if len(ancAst.List)+len(ancAst.Body) == len(anc.node.child) {
					// All case clause children are collected.
					// Split children in condition and body nodest to desambiguify the AST.
					interp.nindex++
					body := &Node{anc: anc.node, interp: interp, index: interp.nindex, pos: pos, kind: CaseBody, action: Nop, val: &i, gen: nop}
					body.child = append(body.child, anc.node.child[len(ancAst.List):]...)
					for i := range body.child {
						body.child[i].anc = body
					}
					anc.node.child = append(anc.node.child[:len(ancAst.List)], body)
				}
			}
		}
		return n
	}

	// Populate our own private AST from Go parser AST.
	// A stack of ancestor nodes is used to keep track of current ancestor for each depth level
	ast.Inspect(f, func(node ast.Node) bool {
		anc = st.top()
		var pos token.Pos
		if node != nil {
			pos = node.Pos()
		}
		switch a := node.(type) {
		case nil:
			anc = st.pop()

		case *ast.ArrayType:
			st.push(addChild(&root, anc, pos, ArrayType, Nop), node)

		case *ast.AssignStmt:
			var action Action
			var kind Kind
			if len(a.Lhs) > 1 && len(a.Rhs) == 1 {
				if a.Tok == token.DEFINE {
					kind = DefineX
				} else {
					kind = AssignXStmt
				}
				action = AssignX
			} else {
				nbAssign = len(a.Lhs)
				kind = AssignStmt
				switch a.Tok {
				case token.ASSIGN:
					action = Assign
				case token.ADD_ASSIGN:
					action = AddAssign
				case token.AND_ASSIGN:
					action = AndAssign
				case token.AND_NOT_ASSIGN:
					action = AndNotAssign
				case token.DEFINE:
					kind = Define
					action = Assign
				case token.SHL_ASSIGN:
					action = ShlAssign
				case token.SHR_ASSIGN:
					action = ShrAssign
				case token.MUL_ASSIGN:
					action = MulAssign
				case token.OR_ASSIGN:
					action = OrAssign
				case token.QUO_ASSIGN:
					action = QuoAssign
				case token.REM_ASSIGN:
					action = RemAssign
				case token.SUB_ASSIGN:
					action = SubAssign
				case token.XOR_ASSIGN:
					action = XorAssign
				}
			}
			st.push(addChild(&root, anc, pos, kind, action), node)

		case *ast.BasicLit:
			n := addChild(&root, anc, pos, BasicLit, Nop)
			n.ident = a.Value
			switch a.Kind {
			case token.CHAR:
				n.val = rune(a.Value[1])
			case token.FLOAT:
				n.val, _ = strconv.ParseFloat(a.Value, 64)
			case token.IMAG:
				v, _ := strconv.ParseFloat(a.Value[:len(a.Value)-1], 64)
				n.val = complex(0, v)
			case token.INT:
				v, _ := strconv.ParseInt(a.Value, 0, 0)
				n.val = int(v)
			case token.STRING:
				n.val = a.Value[1 : len(a.Value)-1]
			}
			st.push(n, node)

		case *ast.BinaryExpr:
			kind := BinaryExpr
			action := Nop
			switch a.Op {
			case token.ADD:
				action = Add
			case token.AND:
				action = And
			case token.AND_NOT:
				action = AndNot
			case token.EQL:
				action = Equal
			case token.GEQ:
				action = GreaterEqual
			case token.GTR:
				action = Greater
			case token.LAND:
				kind = LandExpr
				action = Land
			case token.LOR:
				kind = LorExpr
				action = Lor
			case token.LEQ:
				action = LowerEqual
			case token.LSS:
				action = Lower
			case token.MUL:
				action = Mul
			case token.NEQ:
				action = NotEqual
			case token.OR:
				action = Or
			case token.REM:
				action = Rem
			case token.SUB:
				action = Sub
			case token.SHL:
				action = Shl
			case token.SHR:
				action = Shr
			case token.QUO:
				action = Quo
			case token.XOR:
				action = Xor
			}
			st.push(addChild(&root, anc, pos, kind, action), node)

		case *ast.BlockStmt:
			st.push(addChild(&root, anc, pos, BlockStmt, Nop), node)

		case *ast.BranchStmt:
			var kind Kind
			switch a.Tok {
			case token.BREAK:
				kind = Break
			case token.CONTINUE:
				kind = Continue
			case token.FALLTHROUGH:
				kind = Fallthrough
			case token.GOTO:
				kind = Goto
			}
			st.push(addChild(&root, anc, pos, kind, Nop), node)

		case *ast.CallExpr:
			st.push(addChild(&root, anc, pos, CallExpr, Call), node)

		case *ast.CaseClause:
			st.push(addChild(&root, anc, pos, CaseClause, Case), node)

		case *ast.ChanType:
			st.push(addChild(&root, anc, pos, ChanType, Nop), node)

		case *ast.CommClause:
			st.push(addChild(&root, anc, pos, CommClause, Nop), node)

		case *ast.CompositeLit:
			st.push(addChild(&root, anc, pos, CompositeLitExpr, CompositeLit), node)

		case *ast.DeclStmt:
			st.push(addChild(&root, anc, pos, DeclStmt, Nop), node)

		case *ast.DeferStmt:
			st.push(addChild(&root, anc, pos, DeferStmt, Defer), node)

		case *ast.Ellipsis:
			st.push(addChild(&root, anc, pos, Ellipsis, Nop), node)

		case *ast.ExprStmt:
			st.push(addChild(&root, anc, pos, ExprStmt, Nop), node)

		case *ast.Field:
			st.push(addChild(&root, anc, pos, Field, Nop), node)

		case *ast.FieldList:
			st.push(addChild(&root, anc, pos, FieldList, Nop), node)

		case *ast.File:
			pkgName = a.Name.Name
			st.push(addChild(&root, anc, pos, File, Nop), node)

		case *ast.ForStmt:
			// Disambiguate variants of FOR statements with a node kind per variant
			var kind Kind
			if a.Cond == nil {
				if a.Init != nil && a.Post != nil {
					kind = For3a
				} else {
					kind = For0
				}
			} else {
				switch {
				case a.Init == nil && a.Post == nil:
					kind = For1
				case a.Init != nil && a.Post == nil:
					kind = For2
				case a.Init == nil && a.Post != nil:
					kind = For3
				default:
					kind = For4
				}
			}
			st.push(addChild(&root, anc, pos, kind, Nop), node)

		case *ast.FuncDecl:
			n := addChild(&root, anc, pos, FuncDecl, Nop)
			if a.Recv == nil {
				// function is not a method, create an empty receiver list
				addChild(&root, astNode{n, node}, pos, FieldList, Nop)
			}
			st.push(n, node)

		case *ast.FuncLit:
			n := addChild(&root, anc, pos, FuncLit, GetFunc)
			addChild(&root, astNode{n, node}, pos, FieldList, Nop)
			addChild(&root, astNode{n, node}, pos, Undef, Nop)
			st.push(n, node)

		case *ast.FuncType:
			st.push(addChild(&root, anc, pos, FuncType, Nop), node)

		case *ast.GenDecl:
			var kind Kind
			switch a.Tok {
			case token.CONST:
				kind = ConstDecl
			case token.IMPORT:
				kind = ImportDecl
			case token.TYPE:
				kind = TypeDecl
			case token.VAR:
				kind = VarDecl
			}
			st.push(addChild(&root, anc, pos, kind, Nop), node)

		case *ast.GoStmt:
			st.push(addChild(&root, anc, pos, GoStmt, Nop), node)

		case *ast.Ident:
			n := addChild(&root, anc, pos, Ident, Nop)
			n.ident = a.Name
			st.push(n, node)

		case *ast.IfStmt:
			// Disambiguate variants of IF statements with a node kind per variant
			var kind Kind
			switch {
			case a.Init == nil && a.Else == nil:
				kind = If0
			case a.Init == nil && a.Else != nil:
				kind = If1
			case a.Else == nil:
				kind = If2
			default:
				kind = If3
			}
			st.push(addChild(&root, anc, pos, kind, Nop), node)

		case *ast.ImportSpec:
			st.push(addChild(&root, anc, pos, ImportSpec, Nop), node)

		case *ast.IncDecStmt:
			var action Action
			switch a.Tok {
			case token.INC:
				action = Inc
			case token.DEC:
				action = Dec
			}
			st.push(addChild(&root, anc, pos, IncDecStmt, action), node)

		case *ast.IndexExpr:
			st.push(addChild(&root, anc, pos, IndexExpr, GetIndex), node)

		case *ast.InterfaceType:
			st.push(addChild(&root, anc, pos, InterfaceType, Nop), node)

		case *ast.KeyValueExpr:
			st.push(addChild(&root, anc, pos, KeyValueExpr, Nop), node)

		case *ast.MapType:
			st.push(addChild(&root, anc, pos, MapType, Nop), node)

		case *ast.ParenExpr:
			st.push(addChild(&root, anc, pos, ParenExpr, Nop), node)

		case *ast.RangeStmt:
			// Insert a missing ForRangeStmt for AST correctness
			n := addChild(&root, anc, pos, ForRangeStmt, Nop)
			st.push(addChild(&root, astNode{n, node}, pos, RangeStmt, Range), node)

		case *ast.ReturnStmt:
			st.push(addChild(&root, anc, pos, ReturnStmt, Return), node)

		case *ast.SelectStmt:
			st.push(addChild(&root, anc, pos, SelectStmt, Nop), node)

		case *ast.SelectorExpr:
			st.push(addChild(&root, anc, pos, SelectorExpr, GetIndex), node)

		case *ast.SendStmt:
			st.push(addChild(&root, anc, pos, SendStmt, Send), node)

		case *ast.SliceExpr:
			if a.Low == nil {
				st.push(addChild(&root, anc, pos, SliceExpr, Slice0), node)
			} else {
				st.push(addChild(&root, anc, pos, SliceExpr, Slice), node)
			}

		case *ast.StarExpr:
			st.push(addChild(&root, anc, pos, StarExpr, Star), node)

		case *ast.StructType:
			st.push(addChild(&root, anc, pos, StructType, Nop), node)

		case *ast.SwitchStmt:
			st.push(addChild(&root, anc, pos, Switch, Nop), node)

		case *ast.TypeAssertExpr:
			st.push(addChild(&root, anc, pos, TypeAssertExpr, TypeAssert), node)

		case *ast.TypeSpec:
			st.push(addChild(&root, anc, pos, TypeSpec, Nop), node)

		case *ast.UnaryExpr:
			var kind = UnaryExpr
			var action Action
			switch a.Op {
			case token.AND:
				kind = Address
				action = Addr
			case token.ARROW:
				action = Recv
			case token.NOT:
				action = Not
			case token.SUB:
				action = Negate
			}
			st.push(addChild(&root, anc, pos, kind, action), node)

		case *ast.ValueSpec:
			kind := ValueSpec
			action := Nop
			if a.Values != nil {
				if len(a.Names) > 1 && len(a.Values) == 1 {
					if anc.node.kind == ConstDecl || anc.node.kind == VarDecl {
						kind = DefineX
					} else {
						kind = AssignXStmt
					}
					action = AssignX
				} else {
					if anc.node.kind == ConstDecl || anc.node.kind == VarDecl {
						kind = Define
					} else {
						kind = AssignStmt
					}
					action = Assign
					nbAssign = len(a.Names)
				}
				if a.Type != nil {
					typeSpec = true
				}
			} else if anc.node.kind == ConstDecl {
				kind, action = Define, Assign
			}
			st.push(addChild(&root, anc, pos, kind, action), node)

		default:
			err = AstError(fmt.Errorf("ast: %T not implemented, line %s", a, fset.Position(pos)))
			return false
		}
		return true
	})
	if inFunc {
		// Incremental parsing: statements were inserted in a pseudo function.
		// Return function body as AST root, so its statements are evaluated in global scope
		root.child[1].child[3].anc = nil
		return "_", root.child[1].child[3], err
	}
	return pkgName, root, err
}

type astNode struct {
	node *Node
	ast  ast.Node
}

type nodestack []astNode

func (s *nodestack) push(n *Node, a ast.Node) {
	*s = append(*s, astNode{n, a})
}

func (s *nodestack) pop() astNode {
	l := len(*s) - 1
	res := (*s)[l]
	*s = (*s)[:l]
	return res
}

func (s *nodestack) top() (v astNode) {
	l := len(*s)
	if l > 0 {
		return (*s)[l-1]
	}
	return
}
