package main

// Values of named native types (json.Delim, time.Duration, time.Month) as
// keys of a map[any]T: the same value made by the program and given by
// native code is the same key, as in Go. A value from native code has the
// descriptor of the native type, a value of the program the one of its
// go/types type: the map did not find json.Delim('[') with a tok of
// json.Decoder.Token.

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type ctxKey string

func main() {
	m := map[any]string{
		json.Delim('['): "open",
		json.Delim(']'): "close",
		time.Second:     "second",
		time.March:      "march",
		"plain":         "string",
	}

	dec := json.NewDecoder(strings.NewReader(`[1]`))
	tok, _ := dec.Token()
	fmt.Println(m[tok], tok == json.Delim('['))
	dec.Token()
	tok, _ = dec.Token()
	fmt.Println(m[tok])

	var sm sync.Map
	sm.Store("d", time.Second)
	d, _ := sm.Load("d")
	fmt.Println(m[d], d == time.Second)

	var av atomic.Value
	av.Store(time.March)
	fmt.Println(m[av.Load()])

	r := reflect.ValueOf(time.Second).Interface()
	fmt.Println(m[r])

	ctx := context.WithValue(context.Background(), ctxKey("k"), time.March)
	fmt.Println(m[ctx.Value(ctxKey("k"))])

	// keys added from native values, found with values of the program
	m2 := map[any]int{}
	m2[d]++
	m2[time.Second]++
	m2[tok]++
	m2[json.Delim(']')]++
	fmt.Println(len(m2), m2[time.Second], m2[json.Delim(']')])

	// a switch on the value
	switch d {
	case time.Second:
		fmt.Println("switch: second")
	default:
		fmt.Println("switch: other")
	}

	// iteration gives the keys back
	var ks []string
	for k := range m2 {
		ks = append(ks, fmt.Sprintf("%T=%v", k, k))
	}
	sort.Strings(ks)
	fmt.Println(ks)
	delete(m2, d)
	fmt.Println(len(m2), m2[time.Second])
}
