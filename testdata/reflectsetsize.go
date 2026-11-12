// reflect SetInt and SetUint cut the value to the size of the kind, like Go.
// The checks compare values in the program (==, map keys, Int, Uint): fmt
// boxes a value for native code, and the boxing hides a value that was not cut.
package main

import (
	"fmt"
	"reflect"
)

type small struct {
	I8  int8
	I16 int16
	I32 int32
	I   int
	U8  uint8
	U16 uint16
	U32 uint32
	U   uint
	P   uintptr
}

type myInt8 int8

func main() {
	var s small
	v := reflect.ValueOf(&s).Elem()
	big := int64(1)<<40 + 1<<20 + 300
	for i := 0; i < 4; i++ {
		v.Field(i).SetInt(big)
	}
	for i := 4; i < 9; i++ {
		v.Field(i).SetUint(uint64(big))
	}

	fmt.Println("int8", s.I8 == int8(big), v.Field(0).Int(), reflect.ValueOf(s.I8).Int())
	fmt.Println("int16", s.I16 == int16(big), v.Field(1).Int(), reflect.ValueOf(s.I16).Int())
	fmt.Println("int32", s.I32 == int32(big), v.Field(2).Int(), reflect.ValueOf(s.I32).Int())
	fmt.Println("int", s.I == int(big), v.Field(3).Int() == int64(int(big)))
	fmt.Println("uint8", s.U8 == uint8(big), v.Field(4).Uint(), reflect.ValueOf(s.U8).Uint())
	fmt.Println("uint16", s.U16 == uint16(big), v.Field(5).Uint(), reflect.ValueOf(s.U16).Uint())
	fmt.Println("uint32", s.U32 == uint32(big), v.Field(6).Uint(), reflect.ValueOf(s.U32).Uint())
	fmt.Println("uint", s.U == uint(big), v.Field(7).Uint() == uint64(uint(big)))
	fmt.Println("uintptr", s.P == uintptr(big), v.Field(8).Uint() == uint64(uintptr(big)))

	m := map[int8]string{44: "ok"}
	var x int8
	reflect.ValueOf(&x).Elem().SetInt(300)
	fmt.Println("map key", m[x], x == 44, x+1 == 45)

	var y myInt8
	reflect.ValueOf(&y).Elem().SetInt(-129)
	fmt.Println("named", y == 127, int(y))

	var z uint16
	reflect.ValueOf(&z).Elem().SetUint(1<<16 + 5)
	fmt.Println("uint16 var", z == 5, z*2 == 10)

	var w int
	b3 := int64(1)<<40 + 3
	reflect.ValueOf(&w).Elem().SetInt(b3)
	fmt.Println("int var", w == int(b3), int64(w) == b3, map[int]bool{int(b3): true}[w])
}
