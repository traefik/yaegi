package main

import (
	"fmt"
	"reflect"
)

// recover in a deferred function called through a method wrapper (method
// value, method expression, interface method, reflect method) stops the
// panic, as in gc. So does "defer recover()" in a deferred function, and
// recover in a function made by reflect.MakeFunc and deferred directly; it
// does not stop it in a function called by that one.

type T struct{ name string }

func (t *T) M() { fmt.Println(t.name, recover()) }

type V int

func (v V) M() { fmt.Println("V", int(v), recover()) }

type I interface{ M() }

func methodValue() { f := (&T{"value"}).M; defer f(); panic(1) }
func methodExpr()  { f := (*T).M; defer f(&T{"expr"}); panic(2) }
func valueMethod() { f := V(3).M; defer f(); panic(3) }
func ifaceMethod() { var i I = V(4); defer i.M(); panic(4) }
func reflectValue() {
	f := reflect.ValueOf(&T{"reflect"}).Method(0).Interface().(func())
	defer f()
	panic(5)
}
func reflectType() {
	f := reflect.TypeOf(V(0)).Method(0).Func.Interface().(func(V))
	defer f(V(6))
	panic(6)
}

func squelch() {
	defer func() { fmt.Println("after squelch", recover()) }()
	defer func() {
		defer recover()
	}()
	panic(7)
}

func notDirect() {
	defer func() { fmt.Println("outer", recover()) }()
	defer func() {
		func() { fmt.Println("nested", recover()) }()
	}()
	panic(8)
}

func makeFunc() {
	f := reflect.MakeFunc(reflect.TypeOf((func())(nil)), func([]reflect.Value) []reflect.Value {
		fmt.Println("made", recover())
		return nil
	}).Interface().(func())
	defer f()
	panic(9)
}

func makeFuncNested() {
	defer func() { fmt.Println("outer", recover()) }()
	inner := reflect.MakeFunc(reflect.TypeOf((func())(nil)), func([]reflect.Value) []reflect.Value {
		fmt.Println("inner", recover())
		return nil
	}).Interface().(func())
	outer := reflect.MakeFunc(reflect.TypeOf((func(func()))(nil)), func(args []reflect.Value) []reflect.Value {
		args[0].Interface().(func())()
		return nil
	}).Interface().(func(func()))
	defer outer(inner)
	panic(10)
}

func main() {
	methodValue()
	methodExpr()
	valueMethod()
	ifaceMethod()
	reflectValue()
	reflectType()
	squelch()
	notDirect()
	makeFunc()
	makeFuncNested()
}
