package interp

import (
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
	BasicLit
	BinaryExpr
	BlockStmt
	BranchStmt
	CallExpr
	CompositeLit
	ExprStmt
	Field
	FieldList
	File
	For0         // for {}
	For1         // for cond {}
	For2         // for init; cond; {}
	For3         // for ; cond; post {}
	For4         // for init; cond; post {}
	ForRangeStmt // for range
	ForStmt
	FuncDecl
	FuncType
	Ident
	If0 // if cond {}
	If1 // if cond {} else {}
	If2 // if init; cond {}
	If3 // if init; cond {} else {}
	IfStmt
	IncDecStmt
	IndexExpr
	ParenExpr
	RangeStmt
	ReturnStmt
)

var kinds = [...]string{
	Undef:        "Undef",
	ArrayType:    "ArrayType",
	AssignStmt:   "AssignStmt",
	BasicLit:     "BasicLit",
	BinaryExpr:   "BinaryExpr",
	BlockStmt:    "BlockStmt",
	BranchStmt:   "BranchStmt",
	CallExpr:     "CallExpr",
	CompositeLit: "CompositLit",
	ExprStmt:     "ExprStmt",
	Field:        "Field",
	FieldList:    "FieldList",
	File:         "File",
	For0:         "For0",
	For1:         "For1",
	For2:         "For2",
	For3:         "For3",
	For4:         "For4",
	ForRangeStmt: "ForRangeStmt",
	ForStmt:      "ForStmt",
	FuncDecl:     "FuncDecl",
	FuncType:     "FuncType",
	Ident:        "Ident",
	If0:          "If0",
	If1:          "If1",
	If2:          "If2",
	If3:          "If3",
	IfStmt:       "IfStmt",
	IncDecStmt:   "IncDecStmt",
	IndexExpr:    "IndexExpr",
	ParenExpr:    "ParenExpr",
	RangeStmt:    "RangeStmt",
	ReturnStmt:   "ReturnStmt",
}

func (k Kind) String() string {
	s := ""
	if 0 <= k && k <= Kind(len(kinds)) {
		s = kinds[k]
	}
	if s == "" {
		s = "kind(" + strconv.Itoa(int(k)) + ")"
	}
	return s
}

// Ast(src) parses src string containing Go code and generates the corresponding AST.
// The AST root node is returned.
func Ast(src string) *Node {
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
		switch node.(type) {
		case nil:
			anc = st.pop()
		case *ast.ArrayType:
			st.push(addChild(&root, anc, &index, ArrayType, &node))
		case *ast.AssignStmt:
			st.push(addChild(&root, anc, &index, AssignStmt, &node))
		case *ast.BasicLit:
			st.push(addChild(&root, anc, &index, BasicLit, &node))
		case *ast.BinaryExpr:
			st.push(addChild(&root, anc, &index, BinaryExpr, &node))
		case *ast.BlockStmt:
			st.push(addChild(&root, anc, &index, BlockStmt, &node))
		case *ast.BranchStmt:
			st.push(addChild(&root, anc, &index, BranchStmt, &node))
		case *ast.CallExpr:
			st.push(addChild(&root, anc, &index, CallExpr, &node))
		case *ast.CompositeLit:
			st.push(addChild(&root, anc, &index, CompositeLit, &node))
		case *ast.ExprStmt:
			st.push(addChild(&root, anc, &index, ExprStmt, &node))
		case *ast.Field:
			st.push(addChild(&root, anc, &index, Field, &node))
		case *ast.FieldList:
			st.push(addChild(&root, anc, &index, FieldList, &node))
		case *ast.File:
			st.push(addChild(&root, anc, &index, File, &node))
		case *ast.ForStmt:
			st.push(addChild(&root, anc, &index, ForStmt, &node))
		case *ast.FuncDecl:
			st.push(addChild(&root, anc, &index, FuncDecl, &node))
		case *ast.FuncType:
			st.push(addChild(&root, anc, &index, FuncType, &node))
		case *ast.Ident:
			st.push(addChild(&root, anc, &index, Ident, &node))
		case *ast.IfStmt:
			st.push(addChild(&root, anc, &index, IfStmt, &node))
		case *ast.IncDecStmt:
			st.push(addChild(&root, anc, &index, IncDecStmt, &node))
		case *ast.IndexExpr:
			st.push(addChild(&root, anc, &index, IndexExpr, &node))
		case *ast.ParenExpr:
			st.push(addChild(&root, anc, &index, ParenExpr, &node))
		case *ast.RangeStmt:
			// Insert a missing ForRangeStmt for AST correctness
			n := addChild(&root, anc, &index, ForRangeStmt, nil)
			st.push(addChild(&root, n, &index, RangeStmt, &node))
		case *ast.ReturnStmt:
			st.push(addChild(&root, anc, &index, ReturnStmt, &node))
		default:
			st.push(addChild(&root, anc, &index, Undef, &node))
		}
		return true
	})
	return root
}

func addChild(root **Node, anc *Node, index *int, kind Kind, anode *ast.Node) *Node {
	*index++
	var i interface{}
	n := &Node{anc: anc, index: *index, kind: kind, anode: anode, val: &i}
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
