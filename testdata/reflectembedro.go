package main

// An exported field reached through an unexported embedded struct is not
// read-only (gc's flagEmbedRO does not stick): openai-go embeds unexported
// structs in its parameter types.

import (
	"fmt"
	"reflect"
)

type inner struct{ A int }

type Outer struct {
	inner
	B int
}

func main() {
	v := reflect.ValueOf(Outer{inner{1}, 2})
	emb := v.Field(0)
	a := emb.Field(0)
	fmt.Println(emb.CanInterface(), a.CanInterface(), a.Interface())
	n := reflect.New(a.Type()).Elem()
	n.Set(a)
	fmt.Println(n.Int())
	p := reflect.ValueOf(&Outer{}).Elem()
	p.Field(0).Field(0).SetInt(7)
	fmt.Println(p.Field(0).Field(0).CanSet(), p.Interface())
}
