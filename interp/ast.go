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
	Switch0 // switch tag {}
	Switch1 // switch init; tag {}
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
	Switch0:          "Switch0",
	Switch1:          "Switch1",
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
	And
	Call
	Case
	CompositeLit
	Dec
	Defer
	Equal
	Greater
	GetFunc
	GetIndex
	Inc
	Land
	Lor
	Lower
	Mul
	Negate
	Not
	NotEqual
	Quotient
	Range
	Recv
	Remain
	Return
	Select
	Send
	Slice
	Slice0
	Star
	Sub
	TypeAssert
)

var actions = [...]string{
	Nop:          "nop",
	Addr:         "&",
	Assign:       "=",
	AssignX:      "X=",
	Add:          "+",
	And:          "&",
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
	Negate:       "-",
	Not:          "!",
	NotEqual:     "!=",
	Quotient:     "/",
	Range:        "range",
	Recv:         "<-",
	Remain:       "%",
	Return:       "return",
	Send:         "<~",
	Slice:        "slice",
	Slice0:       "slice0",
	Star:         "*",
	Sub:          "-",
	TypeAssert:   "TypeAssert",
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

	var root, anc *Node
	var st nodestack
	var nbAssign int
	var typeSpec bool
	var pkgName string

	addChild := func(root **Node, anc *Node, pos token.Pos, kind Kind, action Action) *Node {
		interp.nindex++
		var i interface{}
		n := &Node{anc: anc, interp: interp, index: interp.nindex, fset: fset, pos: pos, kind: kind, action: action, val: &i, gen: builtin[action]}
		n.start = n
		if anc == nil {
			*root = n
		} else {
			anc.child = append(anc.child, n)
			if anc.action == Assign && nbAssign > 1 {
				if !typeSpec && len(anc.child) == 2*nbAssign {
					// All LHS and RSH assing child are now defined, so split multiple assign
					// statement into single assign statements.
					newAnc := anc.anc
					newChild := []*Node{}
					for i := 0; i < nbAssign; i++ {
						// set new signle assign
						interp.nindex++
						na := &Node{anc: anc.anc, interp: interp, index: interp.nindex, pos: pos, kind: anc.kind, action: anc.action, val: new(interface{}), gen: anc.gen}
						na.start = na
						newChild = append(newChild, na)
						// Set single assign left hand side
						anc.child[i].anc = na
						na.child = append(na.child, anc.child[i])
						// Set single assign right hand side
						anc.child[i+nbAssign].anc = na
						na.child = append(na.child, anc.child[i+nbAssign])
					}
					newAnc.child = newChild
				} else if typeSpec && len(anc.child) == 2*nbAssign+1 {
					// All LHS and RHS assing child are now defined, so split multiple assign
					// statement into single assign statements. Set type for each assignment.
					typeSpec = false
					newAnc := anc.anc
					newChild := []*Node{}
					typeNode := anc.child[nbAssign]
					for i := 0; i < nbAssign; i++ {
						// set new signle assign
						interp.nindex++
						na := &Node{anc: anc.anc, interp: interp, index: interp.nindex, pos: pos, kind: anc.kind, action: anc.action, val: new(interface{}), gen: anc.gen}
						na.start = na
						newChild = append(newChild, na)
						// set new type for this assignment
						interp.nindex++
						nt := &Node{anc: na, interp: interp, ident: typeNode.ident, index: interp.nindex, pos: pos, kind: typeNode.kind, action: typeNode.action, val: new(interface{}), gen: typeNode.gen}
						// Set single assign left hand side
						anc.child[i].anc = na
						na.child = append(na.child, anc.child[i])
						// Set assignment type
						na.child = append(na.child, nt)
						// Set single assign right hand side
						anc.child[i+nbAssign+1].anc = na
						na.child = append(na.child, anc.child[i+nbAssign+1])
					}
					newAnc.child = newChild
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
			st.push(addChild(&root, anc, pos, ArrayType, Nop))

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
				if a.Tok == token.DEFINE {
					kind = Define
				} else {
					kind = AssignStmt
				}
				action = Assign
				nbAssign = len(a.Lhs)
			}
			st.push(addChild(&root, anc, pos, kind, action))

		case *ast.BasicLit:
			n := addChild(&root, anc, pos, BasicLit, Nop)
			n.ident = a.Value
			switch a.Kind {
			case token.CHAR:
				n.val = a.Value[1]
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
			st.push(n)

		case *ast.BinaryExpr:
			kind := Kind(BinaryExpr)
			action := Nop
			switch a.Op {
			case token.ADD:
				action = Add
			case token.AND:
				action = And
			case token.EQL:
				action = Equal
			case token.GTR:
				action = Greater
			case token.LAND:
				kind = LandExpr
				action = Land
			case token.LOR:
				kind = LorExpr
				action = Lor
			case token.LSS:
				action = Lower
			case token.MUL:
				action = Mul
			case token.NEQ:
				action = NotEqual
			case token.REM:
				action = Remain
			case token.SUB:
				action = Sub
			case token.QUO:
				action = Quotient
			}
			st.push(addChild(&root, anc, pos, kind, action))

		case *ast.BlockStmt:
			st.push(addChild(&root, anc, pos, BlockStmt, Nop))

		case *ast.BranchStmt:
			var kind Kind
			switch a.Tok {
			case token.BREAK:
				kind = Break
			case token.CONTINUE:
				kind = Continue
			}
			st.push(addChild(&root, anc, pos, kind, Nop))

		case *ast.CallExpr:
			st.push(addChild(&root, anc, pos, CallExpr, Call))

		case *ast.CaseClause:
			st.push(addChild(&root, anc, pos, CaseClause, Case))

		case *ast.ChanType:
			st.push(addChild(&root, anc, pos, ChanType, Nop))

		case *ast.CommClause:
			st.push(addChild(&root, anc, pos, CommClause, Nop))

		case *ast.CompositeLit:
			st.push(addChild(&root, anc, pos, CompositeLitExpr, CompositeLit))

		case *ast.DeclStmt:
			st.push(addChild(&root, anc, pos, DeclStmt, Nop))

		case *ast.DeferStmt:
			st.push(addChild(&root, anc, pos, DeferStmt, Defer))

		case *ast.Ellipsis:
			st.push(addChild(&root, anc, pos, Ellipsis, Nop))

		case *ast.ExprStmt:
			st.push(addChild(&root, anc, pos, ExprStmt, Nop))

		case *ast.Field:
			st.push(addChild(&root, anc, pos, Field, Nop))

		case *ast.FieldList:
			st.push(addChild(&root, anc, pos, FieldList, Nop))

		case *ast.File:
			pkgName = a.Name.Name
			st.push(addChild(&root, anc, pos, File, Nop))

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
				if a.Init == nil && a.Post == nil {
					kind = For1
				} else if a.Init != nil && a.Post == nil {
					kind = For2
				} else if a.Init == nil && a.Post != nil {
					kind = For3
				} else {
					kind = For4
				}
			}
			st.push(addChild(&root, anc, pos, kind, Nop))

		case *ast.FuncDecl:
			n := addChild(&root, anc, pos, FuncDecl, Nop)
			if a.Recv == nil {
				// function is not a method, create an empty receiver list
				addChild(&root, n, pos, FieldList, Nop)
			}
			st.push(n)

		case *ast.FuncLit:
			n := addChild(&root, anc, pos, FuncLit, GetFunc)
			addChild(&root, n, pos, FieldList, Nop)
			addChild(&root, n, pos, Undef, Nop)
			st.push(n)

		case *ast.FuncType:
			st.push(addChild(&root, anc, pos, FuncType, Nop))

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
			st.push(addChild(&root, anc, pos, kind, Nop))

		case *ast.GoStmt:
			st.push(addChild(&root, anc, pos, GoStmt, Nop))

		case *ast.Ident:
			n := addChild(&root, anc, pos, Ident, Nop)
			n.ident = a.Name
			st.push(n)

		case *ast.IfStmt:
			// Disambiguate variants of IF statements with a node kind per variant
			var kind Kind
			if a.Init == nil && a.Else == nil {
				kind = If0
			} else if a.Init == nil && a.Else != nil {
				kind = If1
			} else if a.Else == nil {
				kind = If2
			} else {
				kind = If3
			}
			st.push(addChild(&root, anc, pos, kind, Nop))

		case *ast.ImportSpec:
			st.push(addChild(&root, anc, pos, ImportSpec, Nop))

		case *ast.IncDecStmt:
			var action Action
			switch a.Tok {
			case token.INC:
				action = Inc
			case token.DEC:
				action = Dec
			}
			st.push(addChild(&root, anc, pos, IncDecStmt, action))

		case *ast.IndexExpr:
			st.push(addChild(&root, anc, pos, IndexExpr, GetIndex))

		case *ast.InterfaceType:
			st.push(addChild(&root, anc, pos, InterfaceType, Nop))

		case *ast.KeyValueExpr:
			st.push(addChild(&root, anc, pos, KeyValueExpr, Nop))

		case *ast.MapType:
			st.push(addChild(&root, anc, pos, MapType, Nop))

		case *ast.ParenExpr:
			st.push(addChild(&root, anc, pos, ParenExpr, Nop))

		case *ast.RangeStmt:
			// Insert a missing ForRangeStmt for AST correctness
			n := addChild(&root, anc, pos, ForRangeStmt, Nop)
			st.push(addChild(&root, n, pos, RangeStmt, Range))

		case *ast.ReturnStmt:
			st.push(addChild(&root, anc, pos, ReturnStmt, Return))

		case *ast.SelectStmt:
			st.push(addChild(&root, anc, pos, SelectStmt, Nop))

		case *ast.SelectorExpr:
			st.push(addChild(&root, anc, pos, SelectorExpr, GetIndex))

		case *ast.SendStmt:
			st.push(addChild(&root, anc, pos, SendStmt, Send))

		case *ast.SliceExpr:
			if a.Low == nil {
				st.push(addChild(&root, anc, pos, SliceExpr, Slice0))
			} else {
				st.push(addChild(&root, anc, pos, SliceExpr, Slice))
			}

		case *ast.StarExpr:
			st.push(addChild(&root, anc, pos, StarExpr, Star))

		case *ast.StructType:
			st.push(addChild(&root, anc, pos, StructType, Nop))

		case *ast.SwitchStmt:
			if a.Init == nil {
				st.push(addChild(&root, anc, pos, Switch0, Nop))
			} else {
				st.push(addChild(&root, anc, pos, Switch1, Nop))
			}

		case *ast.TypeAssertExpr:
			st.push(addChild(&root, anc, pos, TypeAssertExpr, TypeAssert))

		case *ast.TypeSpec:
			st.push(addChild(&root, anc, pos, TypeSpec, Nop))

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
			st.push(addChild(&root, anc, pos, kind, action))

		case *ast.ValueSpec:
			kind := ValueSpec
			action := Nop
			if a.Values != nil {
				if len(a.Names) == 1 && len(a.Values) > 1 {
					if anc.kind == ConstDecl || anc.kind == VarDecl {
						kind = DefineX
					} else {
						kind = AssignXStmt
					}
					action = AssignX
				} else {
					if anc.kind == ConstDecl || anc.kind == VarDecl {
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
			} else if anc.kind == ConstDecl {
				kind, action = Define, Assign
			}
			st.push(addChild(&root, anc, pos, kind, action))

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

type nodestack []*Node

func (s *nodestack) push(v *Node) {
	*s = append(*s, v)
}

func (s *nodestack) pop() *Node {
	l := len(*s) - 1
	res := (*s)[l]
	*s = (*s)[:l]
	return res
}

func (s *nodestack) top() *Node {
	l := len(*s)
	if l > 0 {
		return (*s)[l-1]
	}
	return nil
}
