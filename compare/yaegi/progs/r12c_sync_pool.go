package main

import (
	"bytes"
	"fmt"
	"sync"
)

type Buf struct{ B []byte }

func main() {
	p := sync.Pool{New: func() any { return &Buf{B: make([]byte, 0, 16)} }}
	o := p.Get().(*Buf)
	o.B = append(o.B, 'a')
	p.Put(o)
	bp := sync.Pool{New: func() any { return new(bytes.Buffer) }}
	b := bp.Get().(*bytes.Buffer)
	b.WriteString("hi")
	fmt.Println(len(o.B), b.String())
}
