// reflect calls with program types: Value.Call of program funcs, methods
// by name on value and pointer receivers, variadic CallSlice, MakeFunc,
// Method values, and the panics of wrong calls.
package main

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

type Calc struct{ Base int }

func (c Calc) Add(a, b int) int                     { return c.Base + a + b }
func (c Calc) Join(sep string, xs ...string) string { return strings.Join(xs, sep) }
func (c *Calc) Inc(n int)                           { c.Base += n }
func (c Calc) Div(a, b int) (int, error) {
	if b == 0 {
		return 0, errors.New("division by zero")
	}
	return a / b, nil
}
func (c Calc) Apply(f func(int) int) int { return f(c.Base) }

type Greeter interface{ Greet(string) string }

type English struct{}

func (English) Greet(n string) string { return "Hello " + n }

func sum(xs ...int) (total int) {
	for _, x := range xs {
		total += x
	}
	return
}

func pair(a string, b int) (string, int) { return strings.Repeat(a, b), b * 2 }

func show(vs []reflect.Value) string {
	var parts []string
	for _, v := range vs {
		if v.Kind() == reflect.Interface && v.IsNil() {
			parts = append(parts, "nil")
			continue
		}
		parts = append(parts, fmt.Sprintf("%v(%s)", v.Interface(), v.Type()))
	}
	return "[" + strings.Join(parts, " ") + "]"
}

func try(name string, f func() []reflect.Value) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println(name, "panic:", r)
		}
	}()
	fmt.Println(name, show(f()))
}

func main() {
	c := &Calc{Base: 10}
	cv := reflect.ValueOf(c)
	try("Add", func() []reflect.Value {
		return cv.MethodByName("Add").Call([]reflect.Value{reflect.ValueOf(1), reflect.ValueOf(2)})
	})
	try("Join", func() []reflect.Value {
		return cv.MethodByName("Join").Call([]reflect.Value{reflect.ValueOf("-"), reflect.ValueOf("a"), reflect.ValueOf("b")})
	})
	try("JoinSlice", func() []reflect.Value {
		return cv.MethodByName("Join").CallSlice([]reflect.Value{reflect.ValueOf(","), reflect.ValueOf([]string{"x", "y", "z"})})
	})
	try("Inc", func() []reflect.Value {
		return cv.MethodByName("Inc").Call([]reflect.Value{reflect.ValueOf(5)})
	})
	fmt.Println("base after Inc:", c.Base)
	try("Div", func() []reflect.Value {
		return cv.MethodByName("Div").Call([]reflect.Value{reflect.ValueOf(7), reflect.ValueOf(2)})
	})
	try("Div0", func() []reflect.Value {
		return cv.MethodByName("Div").Call([]reflect.Value{reflect.ValueOf(7), reflect.ValueOf(0)})
	})
	try("Apply", func() []reflect.Value {
		return cv.MethodByName("Apply").Call([]reflect.Value{reflect.ValueOf(func(i int) int { return i * 3 })})
	})
	// Value receiver: Inc is not in the method set.
	vv := reflect.ValueOf(*c)
	fmt.Println(vv.NumMethod(), cv.NumMethod(), vv.MethodByName("Inc").IsValid(), cv.MethodByName("Nope").IsValid())
	for i := 0; i < cv.NumMethod(); i++ {
		fmt.Println(i, cv.Type().Method(i).Name, cv.Method(i).Type())
	}
	// Method expression through the type.
	m, _ := reflect.TypeOf(Calc{}).MethodByName("Add")
	try("MethodExpr", func() []reflect.Value {
		return m.Func.Call([]reflect.Value{reflect.ValueOf(Calc{100}), reflect.ValueOf(1), reflect.ValueOf(1)})
	})
	// Program funcs.
	try("sum", func() []reflect.Value {
		return reflect.ValueOf(sum).Call([]reflect.Value{reflect.ValueOf(1), reflect.ValueOf(2), reflect.ValueOf(3)})
	})
	try("sumEmpty", func() []reflect.Value { return reflect.ValueOf(sum).Call(nil) })
	try("pair", func() []reflect.Value {
		return reflect.ValueOf(pair).Call([]reflect.Value{reflect.ValueOf("ab"), reflect.ValueOf(3)})
	})
	try("closure", func() []reflect.Value {
		n := 0
		f := func() int { n++; return n }
		fv := reflect.ValueOf(f)
		fv.Call(nil)
		return fv.Call(nil)
	})
	// Interface method.
	var g Greeter = English{}
	try("iface", func() []reflect.Value {
		return reflect.ValueOf(g).MethodByName("Greet").Call([]reflect.Value{reflect.ValueOf("Bob")})
	})
	gt := reflect.TypeOf((*Greeter)(nil)).Elem()
	gm, ok := gt.MethodByName("Greet")
	fmt.Println(gm.Name, gm.Type, ok)
	// MakeFunc.
	swap := func(in []reflect.Value) []reflect.Value { return []reflect.Value{in[1], in[0]} }
	var intSwap func(int, int) (int, int)
	reflect.ValueOf(&intSwap).Elem().Set(reflect.MakeFunc(reflect.TypeOf(intSwap), swap))
	fmt.Println(intSwap(1, 2))
	var strSwap func(string, string) (string, string)
	reflect.ValueOf(&strSwap).Elem().Set(reflect.MakeFunc(reflect.TypeOf(strSwap), swap))
	fmt.Println(strSwap("a", "b"))
	var mkCalc func(int) Calc
	reflect.ValueOf(&mkCalc).Elem().Set(reflect.MakeFunc(reflect.TypeOf(mkCalc), func(in []reflect.Value) []reflect.Value {
		return []reflect.Value{reflect.ValueOf(Calc{Base: int(in[0].Int()) * 2})}
	}))
	fmt.Println(mkCalc(21).Add(0, 0))
	var vari func(string, ...int) string
	reflect.ValueOf(&vari).Elem().Set(reflect.MakeFunc(reflect.TypeOf(vari), func(in []reflect.Value) []reflect.Value {
		return []reflect.Value{reflect.ValueOf(fmt.Sprint(in[0].String(), in[1].Len(), in[1].Interface()))}
	}))
	fmt.Println(vari("n"), vari("n", 1, 2))
	var fe func() error
	reflect.ValueOf(&fe).Elem().Set(reflect.MakeFunc(reflect.TypeOf(fe), func(in []reflect.Value) []reflect.Value {
		return []reflect.Value{reflect.Zero(reflect.TypeOf((*error)(nil)).Elem())}
	}))
	fmt.Println(fe())
	// Calls passed to program code as func values.
	fmt.Println(c.Apply(reflect.MakeFunc(reflect.TypeOf(func(int) int { return 0 }), func(in []reflect.Value) []reflect.Value {
		return []reflect.Value{reflect.ValueOf(int(in[0].Int()) + 1000)}
	}).Interface().(func(int) int)))
	// Wrong calls.
	try("fewargs", func() []reflect.Value { return cv.MethodByName("Add").Call([]reflect.Value{reflect.ValueOf(1)}) })
	try("invalid", func() []reflect.Value { return cv.MethodByName("Nope").Call(nil) })
	try("notfunc", func() []reflect.Value { return reflect.ValueOf(3).Call(nil) })
}
