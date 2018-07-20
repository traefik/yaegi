package interp

import (
	"fmt"
	"go/ast"
	"go/parser"
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
	CompositeLitExpr
	ConstDecl
	Continue
	DeclStmt
	Defer
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
	CompositeLitExpr: "CompositeLitExpr",
	ConstDecl:        "ConstDecl",
	Continue:         "Continue",
	DeclStmt:         "DeclStmt",
	Defer:            "Defer",
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

// Action defines the node action to perform at execution
type Action uint

// Node actions for the go language
const (
	Nop Action = iota
	Addr
	ArrayLit
	Assign
	AssignX
	Assign0
	Add
	And
	Call
	CallF
	Case
	CompositeLit
	Dec
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
	ArrayLit:     "arrayLit",
	Assign:       "=",
	AssignX:      "X=",
	Assign0:      "0=",
	Add:          "+",
	And:          "&",
	Call:         "call",
	CallF:        "call",
	Case:         "case",
	CompositeLit: "compositeLit",
	Dec:          "--",
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
	Send:         "<-",
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

// Note: no type analysis is performed at this stage, it is done in pre-order processing
// of CFG, in order to accomodate forward type declarations

// Ast parses src string containing Go code and generates the corresponding AST.
// The AST root node is returned.
func (interp *Interpreter) Ast(src string) (*Node, *NodeMap) {
	def := NodeMap{}
	fset := token.NewFileSet() // positions are relative to fset
	f, err := parser.ParseFile(fset, "sample.go", src, 0)
	if err != nil {
		panic(err)
	}

	index := 0
	var root, anc *Node
	var st nodestack
	var nbAssign int
	var typeSpec bool
	var pkgName string

	addChild := func(root **Node, anc *Node, index *int, kind Kind, action Action) *Node {
		*index++
		var i interface{}
		n := &Node{anc: anc, interp: interp, index: *index, kind: kind, action: action, val: &i, run: builtin[action]}
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
						*index++
						na := &Node{anc: anc.anc, interp: interp, index: *index, kind: anc.kind, action: anc.action, val: new(interface{}), run: anc.run}
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
						*index++
						na := &Node{anc: anc.anc, interp: interp, index: *index, kind: anc.kind, action: anc.action, val: new(interface{}), run: anc.run}
						na.start = na
						newChild = append(newChild, na)
						// set new type for this assignment
						*index++
						nt := &Node{anc: na, interp: interp, ident: typeNode.ident, index: *index, kind: typeNode.kind, action: typeNode.action, val: new(interface{}), run: typeNode.run}
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
	// A stack of ancestor nodes is used to keep track of curent ancestor for each depth level
	ast.Inspect(f, func(node ast.Node) bool {
		anc = st.top()
		switch a := node.(type) {
		case nil:
			anc = st.pop()

		case *ast.ArrayType:
			st.push(addChild(&root, anc, &index, ArrayType, Nop))

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
			st.push(addChild(&root, anc, &index, kind, action))

		case *ast.BasicLit:
			n := addChild(&root, anc, &index, BasicLit, Nop)
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
			action := Action(Nop)
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
			st.push(addChild(&root, anc, &index, kind, action))

		case *ast.BlockStmt:
			st.push(addChild(&root, anc, &index, BlockStmt, Nop))

		case *ast.BranchStmt:
			var kind Kind
			switch a.Tok {
			case token.BREAK:
				kind = Break
			case token.CONTINUE:
				kind = Continue
			}
			st.push(addChild(&root, anc, &index, kind, Nop))

		case *ast.CallExpr:
			st.push(addChild(&root, anc, &index, CallExpr, Call))

		case *ast.CaseClause:
			st.push(addChild(&root, anc, &index, CaseClause, Case))

		case *ast.ChanType:
			st.push(addChild(&root, anc, &index, ChanType, Nop))

		case *ast.CompositeLit:
			st.push(addChild(&root, anc, &index, CompositeLitExpr, Nop))

		case *ast.DeclStmt:
			st.push(addChild(&root, anc, &index, DeclStmt, Nop))

		case *ast.Ellipsis:
			st.push(addChild(&root, anc, &index, Ellipsis, Nop))

		case *ast.ExprStmt:
			st.push(addChild(&root, anc, &index, ExprStmt, Nop))

		case *ast.Field:
			st.push(addChild(&root, anc, &index, Field, Nop))

		case *ast.FieldList:
			st.push(addChild(&root, anc, &index, FieldList, Nop))

		case *ast.File:
			pkgName = a.Name.Name
			st.push(addChild(&root, anc, &index, File, Nop))

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
			st.push(addChild(&root, anc, &index, kind, Nop))

		case *ast.FuncDecl:
			n := addChild(&root, anc, &index, FuncDecl, Nop)
			if a.Recv == nil {
				// function is not a method, create an empty receiver list
				addChild(&root, n, &index, FieldList, Nop)
			}
			// Add func name to definitions
			def[a.Name.Name] = n
			st.push(n)

		case *ast.FuncLit:
			n := addChild(&root, anc, &index, FuncLit, GetFunc)
			addChild(&root, n, &index, FieldList, Nop)
			addChild(&root, n, &index, Undef, Nop)
			st.push(n)

		case *ast.FuncType:
			st.push(addChild(&root, anc, &index, FuncType, Nop))

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
			st.push(addChild(&root, anc, &index, kind, Nop))

		case *ast.GoStmt:
			st.push(addChild(&root, anc, &index, GoStmt, Nop))

		case *ast.Ident:
			n := addChild(&root, anc, &index, Ident, Nop)
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
			st.push(addChild(&root, anc, &index, kind, Nop))

		case *ast.ImportSpec:
			st.push(addChild(&root, anc, &index, ImportSpec, Nop))

		case *ast.IncDecStmt:
			var action Action
			switch a.Tok {
			case token.INC:
				action = Inc
			case token.DEC:
				action = Dec
			}
			st.push(addChild(&root, anc, &index, IncDecStmt, action))

		case *ast.IndexExpr:
			st.push(addChild(&root, anc, &index, IndexExpr, GetIndex))

		case *ast.InterfaceType:
			st.push(addChild(&root, anc, &index, InterfaceType, Nop))

		case *ast.KeyValueExpr:
			st.push(addChild(&root, anc, &index, KeyValueExpr, Nop))

		case *ast.MapType:
			st.push(addChild(&root, anc, &index, MapType, Nop))

		case *ast.ParenExpr:
			st.push(addChild(&root, anc, &index, ParenExpr, Nop))

		case *ast.RangeStmt:
			// Insert a missing ForRangeStmt for AST correctness
			n := addChild(&root, anc, &index, ForRangeStmt, Nop)
			st.push(addChild(&root, n, &index, RangeStmt, Range))

		case *ast.ReturnStmt:
			st.push(addChild(&root, anc, &index, ReturnStmt, Return))

		case *ast.SelectorExpr:
			st.push(addChild(&root, anc, &index, SelectorExpr, GetIndex))

		case *ast.SendStmt:
			st.push(addChild(&root, anc, &index, SendStmt, Send))

		case *ast.SliceExpr:
			if a.Low == nil {
				st.push(addChild(&root, anc, &index, SliceExpr, Slice0))
			} else {
				st.push(addChild(&root, anc, &index, SliceExpr, Slice))
			}

		case *ast.StarExpr:
			st.push(addChild(&root, anc, &index, StarExpr, Star))

		case *ast.StructType:
			st.push(addChild(&root, anc, &index, StructType, Nop))

		case *ast.SwitchStmt:
			if a.Init == nil {
				st.push(addChild(&root, anc, &index, Switch0, Nop))
			} else {
				st.push(addChild(&root, anc, &index, Switch1, Nop))
			}

		case *ast.TypeAssertExpr:
			st.push(addChild(&root, anc, &index, TypeAssertExpr, TypeAssert))

		case *ast.TypeSpec:
			n := addChild(&root, anc, &index, TypeSpec, Nop)
			// Add type name to definitions
			def[a.Name.Name] = n
			st.push(n)

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
			st.push(addChild(&root, anc, &index, kind, action))

		case *ast.ValueSpec:
			// TODO: export package global var/const in def
			var kind Kind
			var action Action
			if a.Values != nil {
				if len(a.Names) == 1 && len(a.Values) > 1 {
					kind, action = AssignXStmt, AssignX
				} else {
					kind, action = AssignStmt, Assign
					nbAssign = len(a.Names)
				}
				if a.Type != nil {
					typeSpec = true
				}
			} else if anc.kind == ConstDecl {
				kind, action = AssignStmt, Assign
			} else {
				kind, action = ValueSpec, Assign0
			}
			st.push(addChild(&root, anc, &index, kind, action))

		default:
			fmt.Printf("Unknown kind for %T\n", a)
			st.push(addChild(&root, anc, &index, Undef, Nop))
		}
		return true
	})
	return root, &def
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
