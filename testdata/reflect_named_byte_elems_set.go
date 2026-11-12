// The elements of a []B, B a byte type of the program with methods, are
// settable through reflect and through encoding/binary, which writes them
// with SetUint: in AOT mode the twin gave a copy that was not addressable,
// and SetUint panicked.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"reflect"
)

type B byte

func (b B) String() string { return fmt.Sprintf("B%d", byte(b)) }

func main() {
	s := make([]B, 3)
	err := binary.Read(bytes.NewReader([]byte{1, 2, 3}), binary.LittleEndian, s)
	fmt.Println(err, s, []byte{byte(s[0]), byte(s[1]), byte(s[2])})
	reflect.ValueOf(s).Index(0).SetUint(9)
	reflect.ValueOf(s).Index(1).Set(reflect.ValueOf(B(7)))
	reflect.ValueOf(s).Index(2).Addr().Elem().SetUint(5)
	fmt.Println(s, reflect.ValueOf(s).Index(2).CanSet(), reflect.ValueOf(s).Index(0).Interface())
	fmt.Printf("%v %d %x\n", s, s, s)
}
