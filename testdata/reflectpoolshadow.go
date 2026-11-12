package main

// A *reflect.Value kept in a sync.Pool, as fxamacker/cbor's map encoder
// does (encode_map.go: SetIterKey, SetIterValue, SetZero before Put). In
// AOT mode, native sync.Pool gets a shadow of the cell, and the shadow was
// given back as a struct of the fields of reflect.Value instead of the
// program's reflect.Value: "call of reflect.Value.Uint on struct Value".

import (
	"bytes"
	"fmt"
	"reflect"
	"sort"
	"sync"
)

type enc struct {
	kpool, vpool sync.Pool
}

func newEnc(t reflect.Type) *enc {
	return &enc{
		kpool: sync.Pool{New: func() any { rk := reflect.New(t.Key()).Elem(); return &rk }},
		vpool: sync.Pool{New: func() any { rv := reflect.New(t.Elem()).Elem(); return &rv }},
	}
}

func (me *enc) encode(b *bytes.Buffer, v reflect.Value) {
	iterk := me.kpool.Get().(*reflect.Value)
	defer func() {
		iterk.SetZero()
		me.kpool.Put(iterk)
	}()
	iterv := me.vpool.Get().(*reflect.Value)
	defer func() {
		iterv.SetZero()
		me.vpool.Put(iterv)
	}()
	var kv []string
	for iter := v.MapRange(); iter.Next(); {
		iterk.SetIterKey(iter)
		iterv.SetIterValue(iter)
		kv = append(kv, fmt.Sprintf("%v=%v", iterk.Interface(), iterv.Interface()))
	}
	sort.Strings(kv)
	for _, s := range kv {
		b.WriteString(s + ";")
	}
}

func main() {
	m := map[string]string{"a": "1", "b": "2", "c": "3"}
	e := newEnc(reflect.TypeOf(m))
	for i := 0; i < 3; i++ {
		var b bytes.Buffer
		e.encode(&b, reflect.ValueOf(m))
		fmt.Println(b.String())
	}
	m2 := map[uint64]int{1: 10, 2: 20}
	e2 := newEnc(reflect.TypeOf(m2))
	var b bytes.Buffer
	e2.encode(&b, reflect.ValueOf(m2))
	fmt.Println(b.String())
}
