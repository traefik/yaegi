// Map lookups: zero values of every value type for missing keys, comma-ok
// in assignments, if statements and switch, lookups in nil maps, maps of
// funcs called by key, and maps with interface values.
package main

import (
	"fmt"
	"sort"
	"strings"
)

type S struct {
	A int
	B string
	C []int
}

func main() {
	mi := map[string]int{"a": 1}
	ms := map[string]string{}
	mf := map[string]float64{}
	mb := map[string]bool{}
	mst := map[string]S{}
	msl := map[string][]int{}
	mm := map[string]map[int]int{}
	mp := map[string]*S{}
	ma := map[string]any{}
	mfn := map[string]func() int{}
	mar := map[string][3]int{}
	fmt.Printf("%v %q %v %v %+v %v %v %v %v %v %v\n", mi["x"], ms["x"], mf["x"], mb["x"], mst["x"], msl["x"] == nil, mm["x"] == nil, mp["x"] == nil, ma["x"] == nil, mfn["x"] == nil, mar["x"])

	v, ok := mi["a"]
	fmt.Println(v, ok)
	v, ok = mi["b"]
	fmt.Println(v, ok)
	if v, ok := mi["a"]; ok && v == 1 {
		fmt.Println("found a")
	}
	if _, ok := mi["zz"]; !ok {
		fmt.Println("no zz")
	}
	var x int
	var found bool
	x, found = mi["a"]
	fmt.Println(x, found)
	switch _, ok := ms["k"]; {
	case ok:
		fmt.Println("has k")
	default:
		fmt.Println("no k")
	}

	// Comma-ok into an interface and into a struct field.
	var into struct {
		V  any
		OK bool
	}
	into.V, into.OK = ma["missing"]
	fmt.Println(into)
	ma["n"] = nil
	into.V, into.OK = ma["n"]
	fmt.Println(into, len(ma))

	// Nil map lookups.
	var nm map[int][]string
	l, ok := nm[3]
	fmt.Println(l == nil, ok, len(nm[3]))

	// Map of funcs.
	ops := map[string]func(a, b int) int{
		"add": func(a, b int) int { return a + b },
		"sub": func(a, b int) int { return a - b },
		"mul": func(a, b int) int { return a * b },
	}
	ops["div"] = func(a, b int) int { return a / b }
	names := make([]string, 0)
	for k := range ops {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Print(n, "=", ops[n](12, 4), " ")
	}
	fmt.Println()
	if f, ok := ops["pow"]; !ok {
		fmt.Println("no pow", f == nil)
	}
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		ops["pow"](1, 2)
	}()
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		ops["div"](1, 0)
	}()

	// Closures in a map capturing state.
	counter := 0
	actions := map[string]func(){
		"inc": func() { counter++ },
		"dbl": func() { counter *= 2 },
	}
	for _, a := range strings.Fields("inc inc dbl inc dbl") {
		actions[a]()
	}
	fmt.Println(counter)

	// Map of methods values.
	var sb strings.Builder
	writers := map[int]func(string) (int, error){0: sb.WriteString}
	writers[0]("hello ")
	writers[0]("world")
	fmt.Println(sb.String())

	// Interface values in a map with type switches.
	vals := map[string]any{"i": 1, "s": "str", "f": 2.5, "sl": []int{1}, "m": map[string]int{"k": 1}, "nil": nil}
	keys := make([]string, 0, len(vals))
	for k := range vals {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		switch t := vals[k].(type) {
		case int:
			fmt.Print(k, ":int ", t+1, "; ")
		case string:
			fmt.Print(k, ":string ", len(t), "; ")
		case []int:
			fmt.Print(k, ":slice ", len(t), "; ")
		case nil:
			fmt.Print(k, ":nil; ")
		default:
			fmt.Printf("%s:%T; ", k, t)
		}
	}
	fmt.Println()
	// Type assertion on a missing key.
	_, ok = vals["zzz"].(int)
	fmt.Println(ok)
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		_ = vals["zzz"].(int)
	}()
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		_ = vals["s"].(int)
	}()

	// Modifying a struct in a map needs a copy; arrays too.
	mst["a"] = S{A: 1}
	t := mst["a"]
	t.A = 2
	fmt.Println(mst["a"].A, t.A, len(mst["a"].C))
	mar["a"] = [3]int{1, 2, 3}
	arr := mar["a"]
	arr[0] = 9
	fmt.Println(mar["a"], arr, mar["a"][2])
}
