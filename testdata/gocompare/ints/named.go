// Named integer types: methods with value and pointer receivers that wrap,
// conversions between named and unnamed types, typed constants, and
// arithmetic through interfaces and type switches.
package main

import "fmt"

type Counter uint8
type Delta int16
type ID uint64

func (c *Counter) Inc()               { *c++ }
func (c Counter) Next() Counter       { return c + 1 }
func (c Counter) Add(d Delta) Counter { return Counter(int16(c) + int16(d)) }
func (d Delta) Abs() Delta {
	if d < 0 {
		return -d
	}
	return d
}
func (id ID) Hi() uint32 { return uint32(id >> 32) }
func (id ID) Lo() uint32 { return uint32(id) }

const MaxDelta Delta = 1<<15 - 1

func classify(v any) string {
	switch x := v.(type) {
	case Counter:
		return fmt.Sprint("Counter ", x+1)
	case Delta:
		return fmt.Sprint("Delta ", x*2)
	case uint8:
		return fmt.Sprint("uint8 ", x+1)
	case int:
		return fmt.Sprint("int ", x)
	case ID:
		return fmt.Sprint("ID ", x.Hi(), x.Lo())
	}
	return "other"
}

func main() {
	var c Counter = 254
	c.Inc()
	fmt.Println(c, c.Next(), c.Next().Next())
	c.Inc()
	fmt.Println(c)
	fmt.Println(Counter(10).Add(-20), Counter(250).Add(10), Counter(5).Add(300))
	fmt.Println(MaxDelta, MaxDelta+Delta(c+1), Delta(-32768).Abs(), Delta(-5).Abs())
	id := ID(0xDEADBEEF_CAFEBABE)
	fmt.Println(id.Hi(), id.Lo(), id+ID(id.Lo()), ^id)
	var b uint8 = 255
	fmt.Println(classify(Counter(b)), classify(b), classify(Delta(-20000)), classify(7), classify(id), classify(int8(1)))
	ps := []*Counter{new(Counter), new(Counter)}
	for i := 0; i < 300; i++ {
		ps[i%2].Inc()
	}
	fmt.Println(*ps[0], *ps[1])
	m := map[Counter]Delta{}
	for i := 0; i < 600; i++ {
		m[Counter(i)] += Delta(i)
	}
	fmt.Println(len(m), m[0], m[88], m[255])
	f := Counter.Next
	g := (*Counter).Inc
	var h Counter = 255
	g(&h)
	fmt.Println(f(255), h)
	var iface interface{ Next() Counter } = Counter(255)
	fmt.Println(iface.Next())
	fmt.Printf("%T %v %d %x\n", id, Delta(-1), Counter(200), ID(255))
}
