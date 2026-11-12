package main

import (
	"fmt"
	"reflect"
)

type Message struct {
	Name string
}

var protoMessageType = reflect.TypeOf((*Message)(nil)).Elem()

func main() {
	fmt.Println(protoMessageType.Kind())
}

// Output:
// struct
