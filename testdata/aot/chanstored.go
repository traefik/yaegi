// AOT test program: AOT code stores an interpreted channel in native
// memory with reflect (Value.Set, Value.SetMapIndex). mapstructure builds a
// native map[string]chan int from a struct and SetMapIndex puts the field
// in it (also a slice of channels); json v2 (Go 1.27) copies the value of json.Marshal(ch) into a new
// variable of its type. The VM refused to convert the channel ("converting
// chan vm.Value to native chan int"); now it gets its stand-in, which
// converts back to the same channel.
//
//aot:lib github.com/mitchellh/mapstructure
package main

import (
	"fmt"

	"github.com/mitchellh/mapstructure"
)

type withChan struct {
	C chan int
}

type withChans struct {
	L []chan int
}

func main() {
	ch := make(chan int, 1)
	var m map[string]chan int
	err := mapstructure.Decode(withChan{C: ch}, &m)
	fmt.Println("decode:", err, len(m), m["C"] == ch)
	m["C"] <- 4
	fmt.Println("same channel:", <-ch)

	var n map[string]chan int
	err = mapstructure.Decode(withChan{}, &n)
	fmt.Println("nil channel:", err, len(n), n["C"] == nil)

	var l map[string][]chan int
	err = mapstructure.Decode(withChans{L: []chan int{ch, nil}}, &l)
	fmt.Println("slice:", err, len(l["L"]), l["L"][0] == ch, l["L"][1] == nil)
	l["L"][0] <- 5
	fmt.Println("same channel:", <-ch)
}
