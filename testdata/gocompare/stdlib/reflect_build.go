// reflect building values of program types: New, Zero, Append, AppendSlice,
// MakeSlice, MakeMap, SetMapIndex, MapRange (sorted), MapKeys, Copy,
// StructOf, SliceOf, MapOf, FuncOf, ArrayOf, Convert, and channel ops.
// (The panics of misuse and Value.Cap of a channel are left out: they
// differ in yaegi v2 today.)
package main

import (
	"fmt"
	"reflect"
	"sort"
)

type Key struct {
	A string
	B int
}

type Val struct {
	List []Key
	N    float64
}

type Celsius float64

func (c Celsius) String() string { return fmt.Sprintf("%.1f°C", float64(c)) }

func main() {
	kt := reflect.TypeOf(Key{})
	vt := reflect.TypeOf(Val{})
	p := reflect.New(kt)
	p.Elem().Field(0).SetString("x")
	p.Elem().Field(1).SetInt(5)
	fmt.Printf("%v %+v %T\n", p.Type(), p.Elem().Interface(), p.Interface())
	z := reflect.Zero(vt)
	fmt.Printf("%+v %v\n", z.Interface(), z.CanSet())

	s := reflect.MakeSlice(reflect.SliceOf(kt), 0, 2)
	s = reflect.Append(s, reflect.ValueOf(Key{"a", 1}), reflect.ValueOf(Key{"b", 2}))
	s = reflect.AppendSlice(s, reflect.ValueOf([]Key{{"c", 3}}))
	fmt.Println(s.Len(), s.Interface(), s.Index(2).Field(0))
	dst := reflect.MakeSlice(s.Type(), 2, 2)
	fmt.Println(reflect.Copy(dst, s), dst.Interface())
	s.Index(0).Set(reflect.ValueOf(Key{"z", 26}))
	fmt.Println(s.Slice(0, 2).Interface(), s.Slice3(1, 2, 3).Cap())
	keys0 := []Key{{"p", 1}, {"q", 2}}
	sp := reflect.ValueOf(&keys0).Elem()
	sp.SetLen(1)
	sp.Index(0).Field(1).SetInt(100)
	fmt.Println(keys0, sp.Len(), sp.Cap())

	m := reflect.MakeMap(reflect.MapOf(kt, vt))
	m.SetMapIndex(reflect.ValueOf(Key{"k1", 1}), reflect.ValueOf(Val{N: 1.5}))
	m.SetMapIndex(reflect.ValueOf(Key{"k2", 2}), reflect.ValueOf(Val{List: []Key{{"in", 0}}}))
	m.SetMapIndex(reflect.ValueOf(Key{"k0", 0}), reflect.ValueOf(Val{}))
	m.SetMapIndex(reflect.ValueOf(Key{"k0", 0}), reflect.Value{})
	fmt.Println(m.Len(), m.Interface())
	fmt.Println(m.MapIndex(reflect.ValueOf(Key{"k1", 1})), m.MapIndex(reflect.ValueOf(Key{"no", 9})).IsValid())
	keys := m.MapKeys()
	sort.Slice(keys, func(i, j int) bool { return keys[i].Field(0).String() < keys[j].Field(0).String() })
	for _, k := range keys {
		fmt.Printf("%v => %+v\n", k.Interface(), m.MapIndex(k).Interface())
	}
	var lines []string
	it := m.MapRange()
	for it.Next() {
		lines = append(lines, fmt.Sprintf("%v:%v", it.Key().Interface(), it.Value().Field(1).Float()))
	}
	sort.Strings(lines)
	fmt.Println(lines)
	typed := m.Interface().(map[Key]Val)
	fmt.Println(typed[Key{"k2", 2}].List)
	m.Clear()
	fmt.Println(m.Len())

	// Types built at run time.
	st := reflect.StructOf([]reflect.StructField{
		{Name: "Name", Type: reflect.TypeOf(""), Tag: `json:"name"`},
		{Name: "Temp", Type: reflect.TypeOf(Celsius(0))},
		{Name: "Keys", Type: reflect.SliceOf(kt)},
	})
	sv := reflect.New(st).Elem()
	sv.Field(0).SetString("built")
	sv.Field(1).SetFloat(21.5)
	sv.Field(2).Set(reflect.ValueOf([]Key{{"q", 1}}))
	fmt.Printf("%v | %+v | %s\n", st, sv.Interface(), st.Field(0).Tag.Get("json"))
	at := reflect.ArrayOf(3, reflect.TypeOf(Celsius(0)))
	av := reflect.New(at).Elem()
	av.Index(2).SetFloat(-4)
	fmt.Println(at, av.Interface(), av.Len())
	ft := reflect.FuncOf([]reflect.Type{kt, reflect.TypeOf(0)}, []reflect.Type{reflect.TypeOf("")}, false)
	fmt.Println(ft, ft.NumIn(), ft.Out(0))
	fv := reflect.MakeFunc(ft, func(in []reflect.Value) []reflect.Value {
		k := in[0].Interface().(Key)
		return []reflect.Value{reflect.ValueOf(fmt.Sprint(k.A, k.B*int(in[1].Int())))}
	})
	fmt.Println(fv.Call([]reflect.Value{reflect.ValueOf(Key{"m", 3}), reflect.ValueOf(4)})[0])
	ct := reflect.ChanOf(reflect.BothDir, kt)
	ch := reflect.MakeChan(ct, 2)
	ch.Send(reflect.ValueOf(Key{"c1", 1}))
	fmt.Println(ch.TrySend(reflect.ValueOf(Key{"c2", 2})), ch.TrySend(reflect.ValueOf(Key{"c3", 3})), ch.Len())
	r1, ok1 := ch.Recv()
	r2, ok2 := ch.TryRecv()
	r3, ok3 := ch.TryRecv()
	fmt.Println(r1, ok1, r2, ok2, r3.IsValid(), ok3)
	ch.Close()
	r4, ok4 := ch.Recv()
	fmt.Println(r4, ok4)

	// Convert between program types.
	c := reflect.ValueOf(Celsius(36.6))
	f := c.Convert(reflect.TypeOf(float64(0)))
	back := f.Convert(reflect.TypeOf(Celsius(0)))
	fmt.Println(c, f, back, back.Interface().(fmt.Stringer).String(), c.CanConvert(reflect.TypeOf("")), reflect.ValueOf(65).Convert(reflect.TypeOf("")).Interface())
	type KeyAlias Key
	ka := reflect.ValueOf(Key{"conv", 1}).Convert(reflect.TypeOf(KeyAlias{}))
	fmt.Printf("%T %+v\n", ka.Interface(), ka.Interface())
	sl := reflect.ValueOf([]byte("abc")).Convert(reflect.TypeOf(""))
	arr := reflect.ValueOf([]int{1, 2, 3}).Convert(reflect.TypeOf([3]int{}))
	fmt.Println(sl, arr, arr.Type())
}
