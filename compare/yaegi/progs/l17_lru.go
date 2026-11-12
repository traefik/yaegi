package main

import (
	"fmt"

	lru "github.com/hashicorp/golang-lru"
)

type K struct{ Host string }

func main() {
	c, _ := lru.New(2)
	c.Add(K{"a"}, 1)
	c.Add(K{"b"}, 2)
	c.Get(K{"a"})
	c.Add(K{"c"}, 3)
	fmt.Println(c.Contains(K{"a"}), c.Contains(K{"b"}), c.Len(), c.Keys())
}
