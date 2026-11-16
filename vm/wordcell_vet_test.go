package vm

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// TestWordCellVet is a vet-style check of the seqlock of wordcell.go. A
// cell of a word kind (pointer, map, chan, func, interface, slice) must be
// read with loadCell, loadCellK or loadWord, and written with storeCell or
// storeWord: a plain read can mix two writes of another goroutine, a crash
// of the host. The 3-way dispatch was copied by hand in many places, and a
// missed copy (fmt printing a struct field, a pointer given to native
// code) killed the host.
//
// The test lists every raw access to program memory in the non-test files
// of vm, except wordcell.go:
//
//   - *p, p.R and p.N where p is a *Value (a cell);
//   - x[i] (a read or a write, not &x[i]) and range x with a value, where
//     x is a []Value taken from the R of a value (x, ok := v.R.([]Value),
//     a type switch, sliceVals(v)): the fields of a struct, the elements
//     of an array or a slice.
//
// It compares the counts per function with rawCells below. Each entry was
// reviewed: the cell is not of a word kind there (an int, a struct cell
// whose R does not change), it is new memory that no other goroutine sees
// yet, or it is a register. A new raw access, or one more in a listed
// function, fails the test: use loadCell and storeCell, or, when the
// access is safe, add it here with the reason.
func TestWordCellVet(t *testing.T) {
	if testing.Short() {
		t.Skip("loads the vm package with go/packages")
	}
	cfg := &packages.Config{Mode: packages.NeedName | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedFiles, Dir: "."}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 1 || len(pkgs[0].Errors) > 0 {
		t.Fatalf("load: %v", pkgs)
	}
	got := rawCellAccesses(pkgs[0])
	var bad []string
	for k, n := range got {
		e, ok := rawCells[k]
		switch {
		case !ok:
			bad = append(bad, fmt.Sprintf("%s: %d raw cell accesses, not listed", k, n))
		case n > e.n:
			bad = append(bad, fmt.Sprintf("%s: %d raw cell accesses, %d listed", k, n, e.n))
		}
	}
	for k, e := range rawCells {
		if n := got[k]; n < e.n {
			bad = append(bad, fmt.Sprintf("%s: %d raw cell accesses, %d listed: lower the count", k, n, e.n))
		}
	}
	sort.Strings(bad)
	for _, b := range bad {
		t.Error(b)
	}
	if len(bad) > 0 {
		t.Log("read a cell of a word kind with loadCell and write it with storeCell (wordcell.go); list a safe raw access in rawCells with the reason")
	}
}

// rawCellAccesses counts the raw cell accesses of p per "file.go:func".
func rawCellAccesses(p *packages.Package) map[string]int {
	return rawCellSites(p, nil)
}

// rawCellSites is rawCellAccesses that also calls site for each access.
func rawCellSites(p *packages.Package, site func(key string, pos token.Pos)) map[string]int {
	info := p.TypesInfo
	val := p.Types.Scope().Lookup("Value").Type()
	ptr, slice := types.NewPointer(val), types.NewSlice(val)
	isPtr := func(e ast.Expr) bool {
		tv, ok := info.Types[e]
		return ok && tv.IsValue() && types.Identical(tv.Type, ptr)
	}
	// fromR reports whether e reads the R of a value: v.R or sliceVals(v).
	fromR := func(e ast.Expr) bool {
		switch e := ast.Unparen(e).(type) {
		case *ast.SelectorExpr:
			return e.Sel.Name == "R"
		case *ast.CallExpr:
			id, ok := e.Fun.(*ast.Ident)
			return ok && id.Name == "sliceVals"
		}
		return false
	}
	out := map[string]int{}
	hit := func(key string, pos token.Pos) {
		out[key]++
		if site != nil {
			site(key, pos)
		}
	}
	for _, f := range p.Syntax {
		name := filepath.Base(p.Fset.Position(f.Pos()).Filename)
		if strings.HasSuffix(name, "_test.go") || name == "wordcell.go" {
			continue
		}
		mem := map[types.Object]bool{} // []Value variables taken from an R
		mark := func(lhs ast.Expr) {
			if id, ok := lhs.(*ast.Ident); ok {
				if o := info.ObjectOf(id); o != nil && types.Identical(o.Type(), slice) {
					mem[o] = true
				}
			}
		}
		isMem := func(e ast.Expr) bool {
			id, ok := ast.Unparen(e).(*ast.Ident)
			return ok && mem[info.ObjectOf(id)]
		}
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			key := name + ":" + fd.Name.Name
			addr := map[ast.Node]bool{} // x[i] under &
			ast.Inspect(fd.Body, func(x ast.Node) bool {
				switch e := x.(type) {
				case *ast.AssignStmt:
					for i, r := range e.Rhs {
						if ta, ok := r.(*ast.TypeAssertExpr); ok && fromR(ta.X) {
							if len(e.Rhs) == 1 {
								mark(e.Lhs[0])
							} else {
								mark(e.Lhs[i])
							}
						} else if call, ok := r.(*ast.CallExpr); ok && fromR(call) && i < len(e.Lhs) {
							mark(e.Lhs[i])
						}
					}
				case *ast.ValueSpec:
					for i, r := range e.Values {
						if ta, ok := r.(*ast.TypeAssertExpr); ok && fromR(ta.X) && i < len(e.Names) {
							mark(e.Names[i])
						}
					}
				case *ast.TypeSwitchStmt:
					var x ast.Expr
					switch a := e.Assign.(type) {
					case *ast.AssignStmt:
						x = a.Rhs[0].(*ast.TypeAssertExpr).X
					case *ast.ExprStmt:
						x = a.X.(*ast.TypeAssertExpr).X
					}
					if fromR(x) {
						for _, cc := range e.Body.List {
							if o := info.Implicits[cc]; o != nil && types.Identical(o.Type(), slice) {
								mem[o] = true
							}
						}
					}
				case *ast.UnaryExpr:
					if e.Op == token.AND {
						addr[ast.Unparen(e.X)] = true
					}
				case *ast.StarExpr:
					if isPtr(e.X) {
						hit(key, e.Pos())
					}
				case *ast.SelectorExpr:
					if (e.Sel.Name == "R" || e.Sel.Name == "N") && isPtr(e.X) && !addr[e] {
						hit(key, e.Pos())
					}
				case *ast.IndexExpr:
					if isMem(e.X) && !addr[e] {
						hit(key, e.Pos())
					}
				case *ast.RangeStmt:
					if isMem(e.X) && e.Value != nil {
						hit(key, e.Pos())
					}
				}
				return true
			})
		}
	}
	return out
}

type rawCell struct {
	n   int
	why string
}

// rawCells lists the reviewed raw cell accesses per function (see
// TestWordCellVet).
var rawCells = map[string]rawCell{
	"aggv.go:storeInPlace":          {3, "a struct literal stored in place: storeWord for word fields, plain stores for the others"},
	"aotgeneric.go:callWithShadows": {1, "a new cell, before the program sees it"},
	"callers.go:getUintptr":         {1, "N of a uintptr element"},
	"callers.go:setUintptr":         {1, "N of a uintptr element"},
	"compile.go:globalPtr":          {6, "the init of a global, before the program runs"},
	"decodedst.go:UnmarshalJSON":    {1, "a new cell, before the program sees it"},
	"errors.go:errWalk":             {1, "the result of an Unwrap call: a new slice"},
	"errors.go:errorsAsType":        {2, "a new cell of errors.AsType, not shared"},
	"escape.go:newCell":             {3, "a new cell (allocation)"},
	"escape.go:resetCell":           {4, "a cell reused by escape analysis: not shared"},
	"eval.go:step":                  {1, "the host reads a global between runs"},
	"exec.go:equal":                 {2, "the lengths of string elements: a string is not a word kind"},
	"exec.go:exec":                  {22, "OpLoad, OpStore, OpFieldLoad, OpFieldStore, OpIndexLoad, OpIndexStore: the word kinds take loadWord and storeWord (memSite.word); OpField and OpIndex read a value in a register"},
	"exec.go:runRecv":               {1, "the receiver cell of a go or defer statement: a copy"},
	"exec.go:store":                 {1, "OpStore of any address: storeWord for a word kind (memSite.word)"},
	"features.go:runGoroutine":      {1, "the receiver cell of a go statement: a copy"},
	"flagvars.go:Set":               {1, "a string flag variable: not a word kind"},
	"flagvars.go:String":            {1, "a string flag variable: not a word kind"},
	"flagvars.go:signExtend32":      {2, "N of an int flag variable"},
	"flagvars.go:stringVarHook":     {1, "a string flag variable: not a word kind"},
	"generics.go:sqlNullMethod":     {10, "the fields of a sql.Null receiver value (a register) and N of Valid"},
	"generics.go:sqlNullNative":     {5, "the fields of a sql.Null receiver value (a register)"},
	"generics.go:weakValue":         {1, "the fields of a weak.Pointer value (a register)"},
	"ifaceview.go:firstOf":          {1, "a probe of a struct or array cell (typeTok): not a word kind"},
	"ifaceview.go:hintElems":        {1, "a probe of a struct or array cell: not a word kind"},
	"ifaceview.go:ifaceFrom":        {1, "a probe of a struct or array cell (typeTok): not a word kind"},
	"ifaceview.go:ifaceWord":        {1, "a probe of a struct or array cell: not a word kind"},
	"ifaceview.go:rememberPointee":  {1, "a probe of a struct or array cell: not a word kind"},
	"ifaceview.go:typeOfWord":       {1, "a probe of a struct cell (typeTok): not a word kind"},
	"intrinsics.go:valBytes":        {1, "N of a byte element"},
	"keepmethods.go:keepOmitted":    {1, "a new cell for the IsZero call, before the program sees it"},
	"keepmethods.go:keepValue":      {1, "a new slice for the elements of another representation"},
	"keepmethods.go:keepWrap":       {1, "a new cell for a pointer method, before the program sees it"},
	"linkrand.go:hashBytes":         {2, "N of an integer cell and a []byte probe of runtime.memhash"},
	"linkrand.go:hashString":        {2, "a string cell: not a word kind"},
	"mem.go:assign":                 {5, "a struct or array cell copied as a whole: storeWord for each word element"},
	"mem.go:fieldAddr":              {1, "a probe of a struct cell: not a word kind"},
	"mem.go:indexAddr":              {3, "a probe of an array cell or a slice value: not a word kind"},
	"mem.go:load":                   {2, "Machine.load: loadWord for a word kind (memSite.word)"},
	"mem.go:store":                  {1, "Machine.store: storeWord for a word kind (memSite.word)"},
	"natfast.go:natStrings":         {1, "string elements: not a word kind"},
	"native.go:keepJSONResult":      {1, "the named result of the native call: a local of this goroutine"},
	"native.go:sameVMValue":         {2, "b is a new value of the write-back"},
	"native.go:unpack":              {1, "the results of a call: a new slice"},
	"native.go:unpackRes":           {1, "the results of a call: a new slice"},
	"plugin.go:SetFields":           {1, "a probe of a struct cell: not a word kind"},
	"plugin.go:call":                {2, "the receiver and the results of a host call: copies"},
	"print.go:oneElemData":          {1, "the value of an interface: an Iface does not change"},
	"print.go:runesToStr":           {1, "N of a rune element"},
	"rangefunc.go:rangeFuncPanic":   {2, "N of the state of a range-over-func loop"},
	"rangefunc.go:rangeResumeLoad":  {2, "N of the state of a range-over-func loop"},
	"reflect2.go:hiterFields":       {2, "a probe of a struct cell (typeTok): not a word kind"},
	"reflect2.go:r2SliceCopy":       {4, "the fields of reflect2 slice headers (values)"},
	"reflect2.go:typeOfWord2":       {1, "a probe of a struct cell (typeTok): not a word kind"},
	"slicearr.go:ptrKeyOf":          {2, "a probe of an array or struct cell: not a word kind"},
	"slicesoverlap.go:sliceMem":     {1, "the size of an element, no read"},
	"twin_bridge.go:cellAt":         {1, "a probe of a struct cell: not a word kind"},
	"twin_bridge.go:field":          {1, "a probe of a struct cell: not a word kind"},
	"twin_fallback.go:selectFn":     {4, "the fields of a reflect.SelectCase value (a copy)"},
	"twin_fallback.go:twinArg":      {6, "the fields of a reflect.StructField value (a copy): Type and Index use loadWord"},
	"twin_func.go:Swapper":          {4, "the swapper of a slice whose elements are not a word kind (swapWords for the others)"},
	"twin_func.go:makeFuncP":        {1, "a new cell for an argument"},
	"twin_value.go:Bool":            {1, "N of a bool cell"},
	"twin_value.go:Clear":           {1, "clear of elements that are not a word kind (storeWord for the others)"},
	"twin_value.go:Convert":         {3, "a new cell for the result"},
	"twin_value.go:Int":             {1, "N of an integer cell"},
	"twin_value.go:SetBool":         {1, "a bool cell: not a word kind"},
	"twin_value.go:SetComplex":      {1, "a complex cell: not a word kind"},
	"twin_value.go:SetFloat":        {1, "a float cell: not a word kind"},
	"twin_value.go:SetInt":          {1, "an integer cell: not a word kind"},
	"twin_value.go:SetString":       {1, "a string cell: not a word kind"},
	"twin_value.go:SetUint":         {1, "an integer cell: not a word kind"},
	"twin_value.go:Uint":            {1, "N of an integer cell"},
	"twin_value.go:argNatAt":        {1, "the results of a call: a new slice"},
	"twin_value.go:arrayView":       {1, "the R of an array cell: the view of Elem, set once, or an ordinary array cell, read as cur reads it"},
	"twin_value.go:call":            {3, "new cells for the results"},
	"twin_value.go:cur":             {1, "RV.cur: loadWord for a word kind"},
	"twin_value.go:syncByte":        {1, "N of a byte cell"},
	"unique.go:value":               {2, "the fields of a unique.Handle value (a copy)"},
	"unsafe.go:addrOf":              {1, "a probe of a struct cell: not a word kind"},
	"unsafe.go:arrayAt":             {1, "a probe of an array cell: not a word kind"},
	"unsafe.go:backing":             {1, "a probe of an array cell: not a word kind"},
	"unsafe.go:descend":             {2, "a probe of a struct or array cell: not a word kind"},
	"unsafe.go:reached":             {1, "a probe of an array cell: not a word kind"},
	"unsafe.go:realMemory":          {1, "a probe of a struct cell: not a word kind"},
	"unsafe.go:unsafeToUintptr":     {1, "a probe of a struct or array cell: not a word kind"},
	"unsafe.go:unsafeView":          {1, "a probe of a struct cell: not a word kind"},
	"unsafehdr.go:fromHeader":       {5, "the fields of a header struct value (a copy)"},
	"unsafehdr.go:rewrap":           {1, "the fields of a one-field struct value (a copy)"},
	"unsafehdr.go:viewOf":           {1, "a probe of a struct or array cell: not a word kind"},
	"vmreflect.go:arrayView":        {1, "the R of an array cell: the view of Elem, set once, or an ordinary array cell, read as cur reads it"},
	"vmreflect.go:cur":              {1, "rval.cur: loadWord for a word kind"},
	"vmreflect.go:decV":             {3, "the fields of an encoded reflect.Value (a copy)"},
	"vmreflect.go:intrinsic1":       {11, "N of integer cells, the swapper of elements that are not a word kind, the fields of values, map iterator cells"},
	"vmreflect.go:store":            {1, "rval.store: storeCell when the type is known"},
	"vmreflect.go:zeroCell":         {1, "a new cell"},
	"vmreflect_nat.go:hasNatData":   {2, "the fields of an encoded reflect.Value (a copy)"},
	"vmreflect_nat.go:ifaceCell":    {2, "a new cell"},
	"vmreflect_nat.go:intrinsic":    {2, "the flag of an encoded reflect.Value (a copy)"},
}
