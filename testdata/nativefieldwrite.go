package main

// A struct of funcs stored in a sync.Map comes back as native data, and its
// funcs get the plugin struct as a native value: the writes to its
// unexported fields (e.ptrLevel++, openai-go's fork of encoding/json) must
// work.

import (
	"bytes"
	"fmt"
	"sync"
)

type encodeState struct {
	bytes.Buffer
	ptrLevel uint
	ptrSeen  map[any]struct{}
}

type encoderFunc func(e *encodeState, depth int)

type field struct {
	name    string
	encoder encoderFunc
}

type structFields struct{ list []field }

var fieldCache sync.Map

func enc(e *encodeState, depth int) {
	if e.ptrLevel++; e.ptrLevel > 2 {
		e.ptrSeen[depth] = struct{}{}
		defer delete(e.ptrSeen, depth)
	}
	e.WriteString(fmt.Sprint(depth))
	e.ptrLevel--
}

func cachedFields(k string) structFields {
	if f, ok := fieldCache.Load(k); ok {
		return f.(structFields)
	}
	f, _ := fieldCache.LoadOrStore(k, structFields{list: []field{{"a", enc}, {"b", enc}}})
	return f.(structFields)
}

func main() {
	e := &encodeState{ptrSeen: map[any]struct{}{}}
	for i := 0; i < 2; i++ {
		for _, f := range cachedFields("t").list {
			e.WriteString(f.name)
			f.encoder(e, i)
		}
	}
	fmt.Println(e.String(), e.ptrLevel, len(e.ptrSeen))
}
