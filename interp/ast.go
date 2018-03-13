package interp

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
)

type Kind int

const (
	Undef = iota
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
	Continue
	DeclStmt
	Defer
	ExprStmt
	Fallthrough
	Field
	FieldList
	File
	For0         // for {}
	For1         // for cond {}
	For2         // for init; cond; {}
	For3         // for ; cond; post {}
	For4         // for init; cond; post {}
	ForRangeStmt // for range
	FuncDecl
	FuncType
	GenDecl
	Go
	GoStmt
	Goto
	Ident
	If0 // if cond {}
	If1 // if cond {} else {}
	If2 // if init; cond {}
	If3 // if init; cond {} else {}
	IncDecStmt
	IndexExpr
	LandExpr
	LorExpr
	KeyValueExpr
	MapType
	ParenExpr
	RangeStmt
	ReturnStmt
	SelectorExpr
	SendStmt
	StructType
	Switch0 // switch tag {}
	Switch1 // switch init; tag {}
	TypeSpec
	UnaryExpr
	ValueSpec
)

var kinds = [...]string{
	Undef:            "Undef",
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
	Continue:         "Continue",
	DeclStmt:         "DeclStmt",
	Defer:            "Defer",
	ExprStmt:         "ExprStmt",
	Field:            "Field",
	FieldList:        "FieldList",
	File:             "File",
	For0:             "For0",
	For1:             "For1",
	For2:             "For2",
	For3:             "For3",
	For4:             "For4",
	ForRangeStmt:     "ForRangeStmt",
	FuncDecl:         "FuncDecl",
	FuncType:         "FuncType",
	GenDecl:          "GenDecl",
	Go:               "Go",
	GoStmt:           "GoStmt",
	Goto:             "Goto",
	Ident:            "Ident",
	If0:              "If0",
	If1:              "If1",
	If2:              "If2",
	If3:              "If3",
	IncDecStmt:       "IncDecStmt",
	IndexExpr:        "IndexExpr",
	KeyValueExpr:     "KeyValueExpr",
	LandExpr:         "LandExpr",
	LorExpr:          "LorExpr",
	MapType:          "MapType",
	ParenExpr:        "ParenExpr",
	RangeStmt:        "RangeStmt",
	ReturnStmt:       "ReturnStmt",
	SelectorExpr:     "SelectorExpr",
	SendStmt:         "SendStmt",
	StructType:       "StructType",
	Switch0:          "Switch0",
	Switch1:          "Switch1",
	TypeSpec:         "TypeSpec",
	UnaryExpr:        "UnaryExpr",
	ValueSpec:        "ValueSpec",
}

func (k Kind) String() string {
	if 0 <= k && k <= Kind(len(kinds)) {
		return kinds[k]
	}
	return "Kind(" + strconv.Itoa(int(k)) + ")"
}

type Action int

const (
	Nop = iota
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
	GetIndex
	Inc
	Land
	Lor
	Lower
	Mul
	Range
	Return
	Sub
)

var actions = [...]string{
	Nop:          "nop",
	ArrayLit:     "arrayLit",
	Assign:       "=",
	AssignX:      "=",
	Assign0:      "=",
	Add:          "+",
	And:          "&",
	Call:         "call",
	Case:         "case",
	CompositeLit: "compositeLit",
	Dec:          "--",
	Equal:        "==",
	Greater:      ">",
	GetIndex:     "getIndex",
	Inc:          "++",
	Land:         "&&",
	Lor:          "||",
	Lower:        "<",
	Mul:          "*",
	Range:        "range",
	Return:       "return",
	Sub:          "-",
}

func (a Action) String() string {
	if 0 <= a && a <= Action(len(actions)) {
		return actions[a]
	}
	return "Action(" + strconv.Itoa(int(a)) + ")"
}

// Map of defined symbols (const, variables and functions)
type SymDef map[string]*Node

// Note: no type analysis is performed at this stage, it is done in pre-order processing
// of CFG, in order to accomodate forward type declarations

// Ast(src) parses src string containing Go code and generates the corresponding AST.
// The AST root node is returned.
func Ast(src string, pre SymDef) (*Node, SymDef) {
	var def SymDef
	if pre == nil {
		def = make(map[string]*Node)
	} else {
		def = pre
	}
	fset := token.NewFileSet() // positions are relative to fset
	f, err := parser.ParseFile(fset, "sample.go", src, 0)
	if err != nil {
		panic(err)
	}
	//ast.Print(fset, f)

	index := 0
	var root *Node
	var anc *Node
	var st nodestack
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
				kind, action = AssignXStmt, AssignX
			} else {
				kind, action = AssignStmt, Assign
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
			case token.SUB:
				action = Sub
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
			st.push(addChild(&root, anc, &index, CompositeLitExpr, ArrayLit))

		case *ast.DeclStmt:
			st.push(addChild(&root, anc, &index, DeclStmt, Nop))

		case *ast.ExprStmt:
			st.push(addChild(&root, anc, &index, ExprStmt, Nop))

		case *ast.Field:
			st.push(addChild(&root, anc, &index, Field, Nop))

		case *ast.FieldList:
			st.push(addChild(&root, anc, &index, FieldList, Nop))

		case *ast.File:
			st.push(addChild(&root, anc, &index, File, Nop))

		case *ast.ForStmt:
			// Disambiguate variants of FOR statements with a node kind per variant
			var kind Kind
			if a.Cond == nil {
				kind = For0
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
			// Add func name to definitions
			def[a.Name.Name] = n
			st.push(n)

		case *ast.FuncType:
			st.push(addChild(&root, anc, &index, FuncType, Nop))

		case *ast.GenDecl:
			st.push(addChild(&root, anc, &index, GenDecl, Nop))

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
			st.push(addChild(&root, anc, &index, SendStmt, Nop))

		case *ast.StructType:
			st.push(addChild(&root, anc, &index, StructType, Nop))

		case *ast.SwitchStmt:
			if a.Init == nil {
				st.push(addChild(&root, anc, &index, Switch0, Nop))
			} else {
				st.push(addChild(&root, anc, &index, Switch1, Nop))
			}

		case *ast.TypeSpec:
			st.push(addChild(&root, anc, &index, TypeSpec, Nop))

		case *ast.UnaryExpr:
			st.push(addChild(&root, anc, &index, UnaryExpr, Nop))

		case *ast.ValueSpec:
			var kind Kind
			var action Action
			if a.Values != nil {
				if len(a.Names) == 1 && len(a.Values) > 1 {
					kind, action = AssignXStmt, AssignX
				} else {
					kind, action = AssignStmt, Assign
				}
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
	return root, def
}

func addChild(root **Node, anc *Node, index *int, kind Kind, action Action) *Node {
	*index++
	var i interface{}
	n := &Node{anc: anc, index: *index, kind: kind, action: action, val: &i, run: builtin[action]}
	n.Start = n
	if anc == nil {
		*root = n
	} else {
		anc.Child = append(anc.Child, n)
	}
	return n
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
