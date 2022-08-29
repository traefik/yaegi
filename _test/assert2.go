package main

import (
	"strings"
	"sync"
)

// Define an interface of stringBuilder that is compatible with
// strings.Builder(go 1.10) and bytes.Buffer(< go 1.10).
type stringBuilder interface {
	WriteRune(r rune) (n int, err error)
	WriteString(s string) (int, error)
	Reset()
	Grow(n int)
	String() string
}

var builderPool = sync.Pool{New: func() interface{} {
	return newStringBuilder()
}}

func newStringBuilder() stringBuilder {
	return &strings.Builder{}
}

func main() {
	i := builderPool.Get()
	sb := i.(stringBuilder)
	_, _ = sb.WriteString("hello")

	println(sb.String())

	builderPool.Put(i)
}

// Output:
// hello
