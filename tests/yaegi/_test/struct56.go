package main

import (
	"encoding/json"
	"fmt"
)

type A struct {
	IA InnerA
}

type InnerA struct {
	Timestamp int64
}

func main() {
	a := &A{}
	b, _ := json.Marshal(a)
	fmt.Println(string(b))
}

// Output:
// {"IA":{"Timestamp":0}}
