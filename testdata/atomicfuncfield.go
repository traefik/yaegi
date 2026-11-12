// A struct with a func field kept in an atomic.Value, as the produceHolder
// of the OpenTelemetry metric SDK (ManualReader.Collect): the func that
// comes back is the closure itself, so a pointer argument is the caller's
// variable, and a value of a generic type written through it keeps its type.
package main

import (
	"fmt"
	"sort"
	"sync/atomic"
)

type Sum[N int64 | float64] struct {
	Points []N
	Mono   bool
}

type Hist[N int64 | float64] struct{ Counts []uint64 }

type Agg interface{ agg() }

func (Sum[N]) agg()  {}
func (Hist[N]) agg() {}

type Metrics struct {
	Names []string
	Data  []Agg
}

type pipeline struct{ calls int }

func (p *pipeline) produce(rm *Metrics) error {
	p.calls++
	rm.Names = append(rm.Names[:0], "requests", "path_len")
	rm.Data = append(rm.Data[:0], Sum[int64]{Points: []int64{int64(p.calls)}, Mono: true}, Hist[float64]{Counts: []uint64{1, 0}})
	return nil
}

type produceHolder struct {
	produce func(*Metrics) error
}

type reader struct{ producer atomic.Value }

func (r *reader) register(p *pipeline) bool {
	return r.producer.CompareAndSwap(nil, produceHolder{produce: p.produce})
}

func (r *reader) collect(rm *Metrics) error {
	ph, ok := r.producer.Load().(produceHolder)
	if !ok {
		return fmt.Errorf("invalid producer")
	}
	return ph.produce(rm)
}

func describe(rm *Metrics) string {
	var out []string
	for i, d := range rm.Data {
		switch d := d.(type) {
		case Sum[int64]:
			out = append(out, fmt.Sprintf("%s=sum%v/%v", rm.Names[i], d.Points, d.Mono))
		case Hist[float64]:
			out = append(out, fmt.Sprintf("%s=hist%v", rm.Names[i], d.Counts))
		default:
			out = append(out, rm.Names[i]+"=other")
		}
	}
	sort.Strings(out)
	return fmt.Sprint(out)
}

func main() {
	p := &pipeline{}
	r := &reader{}
	fmt.Println(r.register(p), r.register(p))
	var rm Metrics
	for range 2 {
		fmt.Println(r.collect(&rm), describe(&rm))
	}
	fmt.Println(p.calls)

	// The same func through Store and Swap, and a func value alone.
	var v atomic.Value
	v.Store(produceHolder{produce: p.produce})
	old := v.Swap(produceHolder{produce: func(rm *Metrics) error { rm.Names = []string{"swapped"}; return nil }})
	fmt.Println(old.(produceHolder).produce(&rm), describe(&rm))
	fmt.Println(v.Load().(produceHolder).produce(&rm), rm.Names)
	var f atomic.Value
	f.Store(func(n *int) { *n++ })
	n := 1
	f.Load().(func(*int))(&n)
	fmt.Println(n)
}
