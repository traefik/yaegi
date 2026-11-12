package main

import (
	"bytes"
	"fmt"
	"strings"
)

type S struct {
	Write func([]byte) (int, error)
}

func main() {
	b := &bytes.Buffer{}

	var f func([]byte) (int, error)
	f = b.Write
	_, _ = f([]byte("a"))

	g := b.Write
	_, _ = g([]byte("b"))

	s := S{Write: (&bytes.Buffer{}).Write}
	n, err := s.Write([]byte("cd"))
	fmt.Println(n, err)

	s.Write = b.Write
	_, _ = s.Write([]byte("c"))

	r := strings.NewReplacer("a", "b")
	var h func(string) string
	h = r.Replace

	fmt.Println(b.String(), h("aa"))
}

// Output:
// 2 <nil>
// abc bb
