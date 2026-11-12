package main

import (
	"fmt"

	"github.com/spf13/cast"
)

type Port int

type Name string

func (n Name) String() string { return "name:" + string(n) }

func main() {
	fmt.Println(cast.ToInt("42"), cast.ToInt(Port(8)), cast.ToString(Name("x")))
	fmt.Println(cast.ToStringSlice([]any{"a", 1}), cast.ToBool("true"))
	fmt.Println(cast.ToDuration("1m"))
}
