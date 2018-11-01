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
	Rtype
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
	Rtype:            "Rtype",
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
	Case
	CompositeLit
	Dec
	Equal
	Greater
	GetFunc
	GetIndex
	//Go
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
	Case:         "case",
	CompositeLit: "compositeLit",
	Dec:          "--",
	Equal:        "==",
	Greater:      ">",
	GetFunc:      "getFunc",
	GetIndex:     "getIndex",
	//Go:           "go",
	Inc:        "++",
	Land:       "&&",
	Lor:        "||",
	Lower:      "<",
	Mul:        "*",
	Negate:     "-",
	Not:        "!",
	NotEqual:   "!=",
	Quotient:   "/",
	Range:      "range",
	Recv:       "<-",
	Remain:     "%",
	Return:     "return",
	Send:       "<-",
	Slice:      "slice",
	Slice0:     "slice0",
	Star:       "*",
	Sub:        "-",
	TypeAssert: "TypeAssert",
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
// The package name and the AST root node are returned.
func (interp *Interpreter) Ast(src, name string) (string, *Node) {
	fset := token.NewFileSet() // positions are relative to fset
	f, err := parser.ParseFile(fset, name, src, 0)
	if err != nil {
		panic(err)
	}

	var root, anc *Node
	var st nodestack
	var nbAssign int
	var typeSpec bool
	var pkgName string

	addChild := func(root **Node, anc *Node, kind Kind, action Action) *Node {
		interp.nindex++
		var i interface{}
		n := &Node{anc: anc, interp: interp, index: interp.nindex, kind: kind, action: action, val: &i, gen: builtin[action]}
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
						na := &Node{anc: anc.anc, interp: interp, index: interp.nindex, kind: anc.kind, action: anc.action, val: new(interface{}), gen: anc.gen}
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
						na := &Node{anc: anc.anc, interp: interp, index: interp.nindex, kind: anc.kind, action: anc.action, val: new(interface{}), gen: anc.gen}
						na.start = na
						newChild = append(newChild, na)
						// set new type for this assignment
						interp.nindex++
						nt := &Node{anc: na, interp: interp, ident: typeNode.ident, index: interp.nindex, kind: typeNode.kind, action: typeNode.action, val: new(interface{}), gen: typeNode.gen}
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
			st.push(addChild(&root, anc, ArrayType, Nop))

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
			st.push(addChild(&root, anc, kind, action))

		case *ast.BasicLit:
			n := addChild(&root, anc, BasicLit, Nop)
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
			st.push(addChild(&root, anc, kind, action))

		case *ast.BlockStmt:
			st.push(addChild(&root, anc, BlockStmt, Nop))

		case *ast.BranchStmt:
			var kind Kind
			switch a.Tok {
			case token.BREAK:
				kind = Break
			case token.CONTINUE:
				kind = Continue
			}
			st.push(addChild(&root, anc, kind, Nop))

		case *ast.CallExpr:
			st.push(addChild(&root, anc, CallExpr, Call))

		case *ast.CaseClause:
			st.push(addChild(&root, anc, CaseClause, Case))

		case *ast.ChanType:
			st.push(addChild(&root, anc, ChanType, Nop))

		case *ast.CompositeLit:
			st.push(addChild(&root, anc, CompositeLitExpr, Nop))

		case *ast.DeclStmt:
			st.push(addChild(&root, anc, DeclStmt, Nop))

		case *ast.Ellipsis:
			st.push(addChild(&root, anc, Ellipsis, Nop))

		case *ast.ExprStmt:
			st.push(addChild(&root, anc, ExprStmt, Nop))

		case *ast.Field:
			st.push(addChild(&root, anc, Field, Nop))

		case *ast.FieldList:
			st.push(addChild(&root, anc, FieldList, Nop))

		case *ast.File:
			pkgName = a.Name.Name
			st.push(addChild(&root, anc, File, Nop))

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
			st.push(addChild(&root, anc, kind, Nop))

		case *ast.FuncDecl:
			n := addChild(&root, anc, FuncDecl, Nop)
			if a.Recv == nil {
				// function is not a method, create an empty receiver list
				addChild(&root, n, FieldList, Nop)
			}
			st.push(n)

		case *ast.FuncLit:
			n := addChild(&root, anc, FuncLit, GetFunc)
			addChild(&root, n, FieldList, Nop)
			addChild(&root, n, Undef, Nop)
			st.push(n)

		case *ast.FuncType:
			st.push(addChild(&root, anc, FuncType, Nop))

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
			st.push(addChild(&root, anc, kind, Nop))

		case *ast.GoStmt:
			st.push(addChild(&root, anc, GoStmt, Nop))

		case *ast.Ident:
			n := addChild(&root, anc, Ident, Nop)
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
			st.push(addChild(&root, anc, kind, Nop))

		case *ast.ImportSpec:
			st.push(addChild(&root, anc, ImportSpec, Nop))

		case *ast.IncDecStmt:
			var action Action
			switch a.Tok {
			case token.INC:
				action = Inc
			case token.DEC:
				action = Dec
			}
			st.push(addChild(&root, anc, IncDecStmt, action))

		case *ast.IndexExpr:
			st.push(addChild(&root, anc, IndexExpr, GetIndex))

		case *ast.InterfaceType:
			st.push(addChild(&root, anc, InterfaceType, Nop))

		case *ast.KeyValueExpr:
			st.push(addChild(&root, anc, KeyValueExpr, Nop))

		case *ast.MapType:
			st.push(addChild(&root, anc, MapType, Nop))

		case *ast.ParenExpr:
			st.push(addChild(&root, anc, ParenExpr, Nop))

		case *ast.RangeStmt:
			// Insert a missing ForRangeStmt for AST correctness
			n := addChild(&root, anc, ForRangeStmt, Nop)
			st.push(addChild(&root, n, RangeStmt, Range))

		case *ast.ReturnStmt:
			st.push(addChild(&root, anc, ReturnStmt, Return))

		case *ast.SelectorExpr:
			st.push(addChild(&root, anc, SelectorExpr, GetIndex))

		case *ast.SendStmt:
			st.push(addChild(&root, anc, SendStmt, Send))

		case *ast.SliceExpr:
			if a.Low == nil {
				st.push(addChild(&root, anc, SliceExpr, Slice0))
			} else {
				st.push(addChild(&root, anc, SliceExpr, Slice))
			}

		case *ast.StarExpr:
			st.push(addChild(&root, anc, StarExpr, Star))

		case *ast.StructType:
			st.push(addChild(&root, anc, StructType, Nop))

		case *ast.SwitchStmt:
			if a.Init == nil {
				st.push(addChild(&root, anc, Switch0, Nop))
			} else {
				st.push(addChild(&root, anc, Switch1, Nop))
			}

		case *ast.TypeAssertExpr:
			st.push(addChild(&root, anc, TypeAssertExpr, TypeAssert))

		case *ast.TypeSpec:
			st.push(addChild(&root, anc, TypeSpec, Nop))

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
			st.push(addChild(&root, anc, kind, action))

		case *ast.ValueSpec:
			var kind Kind
			var action Action
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
			} else {
				kind, action = ValueSpec, Assign0
			}
			st.push(addChild(&root, anc, kind, action))

		default:
			fmt.Printf("Unknown kind for %T\n", a)
			st.push(addChild(&root, anc, Undef, Nop))
		}
		return true
	})
	return pkgName, root
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
