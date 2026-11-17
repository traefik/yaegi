// github.com/maypok86/otter and github.com/dolthub/maphash: maphash reads
// the hash function of the runtime type of a map, which the VM gives itself
// (vm/generics.go, dolthubRuntimeHasher).
package main

import (
	"fmt"

	"github.com/dolthub/maphash"
	"github.com/maypok86/otter"
)

type pair struct {
	a string
	b int
}

func main() {
	hs := maphash.NewHasher[string]()
	fmt.Println(hs.Hash("abc") == hs.Hash("abc"), hs.Hash("abc") != hs.Hash("abd"))
	h2 := maphash.NewSeed(hs)
	fmt.Println(h2.Hash("abc") == h2.Hash("abc"), h2.Hash("abc") != hs.Hash("abc"))
	hp := maphash.NewHasher[pair]()
	fmt.Println(hp.Hash(pair{"x", 1}) == hp.Hash(pair{"x", 1}), hp.Hash(pair{"x", 1}) != hp.Hash(pair{"x", 2}))
	hi := maphash.NewHasher[int64]()
	fmt.Println(hi.Hash(7) == hi.Hash(7), hi.Hash(7) != hi.Hash(8))

	c, err := otter.MustBuilder[string, []byte](1000).Build()
	if err != nil {
		panic(err)
	}
	for i := range 500 {
		c.Set(fmt.Sprintf("k%d", i), fmt.Appendf(nil, "v%d", i))
	}
	hits := 0
	for i := range 500 {
		if v, ok := c.Get(fmt.Sprintf("k%d", i)); ok && string(v) == fmt.Sprintf("v%d", i) {
			hits++
		}
	}
	_, ok := c.Get("nope")
	fmt.Println(hits, ok, c.Size())
	c.Delete("k1")
	_, ok = c.Get("k1")
	fmt.Println(ok, c.Size())
	c.Close()
}
