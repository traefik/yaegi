package main

// Standard library API newer than the yaegi release Traefik depends on (the
// native registry is generated for the Go release yaegi v2 is built with), and
// iter.Pull, which the VM implements without runtime coroutines.

import (
	"bytes"
	"crypto/sha3"
	"encoding/hex"
	"fmt"
	"iter"
	"maps"
	"slices"
	"strings"
	"sync"
)

func main() {
	var parts []string
	for p := range strings.SplitSeq("a,b,c", ",") { // Go 1.24
		parts = append(parts, p)
	}
	fmt.Println(parts, len(slices.Collect(strings.Lines("x\ny\n"))))
	for f := range bytes.FieldsSeq([]byte(" one  two ")) {
		fmt.Print(string(f), ";")
	}
	sum := sha3.Sum256([]byte("plugin"))
	fmt.Println(hex.EncodeToString(sum[:4]))

	next, stop := iter.Pull(slices.Values([]int{1, 2, 3}))
	a, ok1 := next()
	b, ok2 := next()
	stop()
	c, ok3 := next()
	fmt.Println(a, ok1, b, ok2, c, ok3)

	next2, stop2 := iter.Pull2(maps.All(map[string]int{"k": 7}))
	defer stop2()
	k, v, ok := next2()
	_, _, end := next2()
	fmt.Println(k, v, ok, end)

	stopped := false
	seq := func(yield func(int) bool) {
		defer func() { stopped = true }()
		for i := 0; ; i++ {
			if !yield(i) {
				return
			}
		}
	}
	n, s := iter.Pull(seq)
	n()
	x, _ := n()
	s()
	fmt.Println(x, stopped)

	once := sync.OnceFunc(func() { fmt.Println("once") })
	defer once()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); once() }()
	wg.Wait()
}
