package main

import (
	"encoding/json"
	"os"
)

type S struct {
	Name  string
	Child []*S
}

func main() {
	a := S{Name: "hello"}
	a.Child = append(a.Child, &S{Name: "world"})
	json.NewEncoder(os.Stdout).Encode(a)
	a.Child[0].Child = append([]*S{}, &S{Name: "sunshine"})
	json.NewEncoder(os.Stdout).Encode(a)
}

// Output:
// {"Name":"hello","Child":[{"Name":"world","Child":null}]}
// {"Name":"hello","Child":[{"Name":"world","Child":[{"Name":"sunshine","Child":null}]}]}
