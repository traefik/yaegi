package main

import (
	"fmt"
	"sort"

	"github.com/fatih/structs"
)

type Inner struct {
	Port int `structs:"port"`
}

type Server struct {
	Name    string `structs:"name"`
	Enabled bool   `structs:"enabled"`
	Inner   `structs:"inner"`
	secret  string
	Opt     *string `structs:"opt,omitempty"`
}

func main() {
	s := Server{Name: "web", Enabled: true, Inner: Inner{8080}, secret: "x"}
	m := structs.Map(s)
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Println(keys, m["name"], m["inner"])
	fmt.Println(structs.Names(s), structs.Name(s), structs.HasZero(s), structs.IsZero(s))
	for _, f := range structs.Fields(&s) {
		fmt.Println(f.Name(), f.Tag("structs"), f.IsExported())
	}
	f := structs.New(&s).Field("Name")
	fmt.Println(f.Set("api"), s.Name)
}
