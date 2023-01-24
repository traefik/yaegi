package main

import (
	"fmt"
)

func MapOf[K comparable, V any](m map[K]V) Map[K, V] {
	return Map[K, V]{m}
}

type Map[K comparable, V any] struct {
	ж map[K]V
}

func (v MapView) Int() Map[string, int] { return MapOf(v.ж.Int) }

type VMap struct {
	Int map[string]int
}

type MapView struct {
	ж *VMap
}

func main() {
	mv := MapView{&VMap{}}
	fmt.Println(mv.ж)
}

// Output:
// &{map[]}
