// Method sets: value and pointer receivers, automatic & and * on calls,
// which types satisfy which interfaces (checked with assertions on any),
// methods on addressable elements (slices, arrays, fields) and on map
// values through a copy.
package main

import "fmt"

type Counter struct{ n int }

func (c Counter) Get() int { return c.n }
func (c *Counter) Inc()    { c.n++ }
func (c Counter) IncCopy() Counter {
	c.n++
	return c
}

type Getter interface{ Get() int }
type Incer interface{ Inc() }
type Both interface {
	Getter
	Incer
}

type Holder struct {
	C  Counter
	CP *Counter
}

func check(name string, v any) {
	_, g := v.(Getter)
	_, i := v.(Incer)
	_, b := v.(Both)
	fmt.Printf("%s: %T Getter=%v Incer=%v Both=%v\n", name, v, g, i, b)
}

func main() {
	var c Counter
	c.Inc() // (&c).Inc()
	c.Inc()
	fmt.Println(c.Get(), c.n)
	pc := &c
	pc.Inc()
	fmt.Println(pc.Get(), (*pc).Get())
	c2 := c.IncCopy()
	fmt.Println(c.n, c2.n)

	check("value", c)
	check("pointer", &c)

	var g Getter = c // copy
	c.Inc()
	fmt.Println(g.Get(), c.Get())
	g = &c // shares
	c.Inc()
	fmt.Println(g.Get())
	var b Both = &c
	b.Inc()
	fmt.Println(b.Get(), c.n)

	// Addressable elements.
	arr := [3]Counter{}
	arr[1].Inc()
	arr[1].Inc()
	sl := []Counter{{}, {}}
	sl[0].Inc()
	for i := range sl {
		sl[i].Inc()
	}
	for _, v := range sl {
		v.Inc() // copy, lost
	}
	fmt.Println(arr, sl)

	h := Holder{CP: &Counter{}}
	h.C.Inc()
	h.CP.Inc()
	h.CP.Inc()
	fmt.Println(h.C.Get(), h.CP.Get())
	hp := &h
	hp.C.Inc()
	fmt.Println(h.C.n)

	m := map[string]Counter{"a": {}}
	tmp := m["a"]
	tmp.Inc()
	m["a"] = tmp
	fmt.Println(m["a"].Get())
	mp := map[string]*Counter{"a": {}}
	mp["a"].Inc()
	mp["a"].Inc()
	fmt.Println(mp["a"].Get())

	// Calls on a composite literal pointer and on a function result.
	fmt.Println((&Counter{5}).Get(), Counter{7}.Get())
	mk := func() *Counter { return &Counter{1} }
	x := mk()
	x.Inc()
	fmt.Println(x.Get(), mk().Get())

	// Nil pointer receiver: the value method panics, a pointer method
	// that does not dereference works.
	var np *Counter
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		fmt.Println(np.Get())
	}()
	var gi Getter = np
	fmt.Println(gi != nil)
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		// Through a call the compiler cannot devirtualize: the message of
		// the method wrapper. A devirtualized call gives a nil dereference.
		fmt.Println(callGet(gi))
	}()
	var safe *Safe
	fmt.Println(safe.Name())
}

//go:noinline
func callGet(g Getter) int { return g.Get() }

type Safe struct{ name string }

func (s *Safe) Name() string {
	if s == nil {
		return "nil Safe"
	}
	return s.name
}
