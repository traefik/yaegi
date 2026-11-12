// The address of an element or a field of a variable of plain data, given
// to native code, has gc's layout: fmt's %p of &a[0] and &a[1] differ by
// the size of the element (GOROOT test fixedbugs/bug260.go), and native
// code writes into the variable itself.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
)

type T1 struct{ x uint8 }

type pair struct {
	a uint8
	b uint32
}

type point struct{ X, Y int16 }

func addr(p any) uint64 {
	a, _ := strconv.ParseUint(fmt.Sprintf("%p", p)[2:], 16, 64)
	return a
}

// deferredNative gives the address of an element to a deferred native call.
func deferredNative() {
	var a [2]int64
	defer func(p *int64, d uint64) { fmt.Println("deferred:", *p, d) }(&a[1], addr(&a[1])-addr(&a[0]))
	defer atomic.AddInt64(&a[1], 2)
	a[0] = 1
}

func main() {
	var b1 [10]T1
	var b2 [4]uint16
	var b4 [3]pair
	fmt.Println(addr(&b1[1])-addr(&b1[0]), addr(&b1[9])-addr(&b1[0]))
	fmt.Println(addr(&b2[3]) - addr(&b2[0]))
	fmt.Println(addr(&b4[1])-addr(&b4[0]), addr(&b4[0].b)-addr(&b4[0].a), addr(&b4[2].b)-addr(&b4[0]))

	// the variable still works as a variable
	b1[3].x = 7
	b2[1] = 500
	b4[2] = pair{1, 70000}
	c := b4
	c[0].a = 9
	fmt.Println(b1[3], b2, b4, c, b4 == c, len(b1))
	for i, p := range b4 {
		fmt.Println(i, p.a, p.b)
	}

	// native code writes into the variable, at once
	var nums [3]int
	n, err := fmt.Sscan("4 5", &nums[0], &nums[2])
	fmt.Println(n, err, nums, addr(&nums[2])-addr(&nums[0]))
	var pts [2]point
	err = json.NewDecoder(strings.NewReader(`{"X": 3, "Y": -4}`)).Decode(&pts[1])
	fmt.Println(err, pts, addr(&pts[1].Y)-addr(&pts[0]))
	fmt.Printf("%v %+v\n", &pts[1], &b4[2])

	// native code that keeps the address of a field writes into the
	// variable later (flag.IntVar)
	var cfg struct {
		N    int
		Rate float64
	}
	fs := flag.NewFlagSet("x", flag.ContinueOnError)
	fs.IntVar(&cfg.N, "n", 1, "")
	fs.Float64Var(&cfg.Rate, "r", 0, "")
	fmt.Println(fs.Parse([]string{"-n", "42", "-r", "0.5"}), cfg.N, cfg.Rate)
	fmt.Println(cfg, addr(&cfg.Rate)-addr(&cfg.N))
	deferredNative()

	// This program has its own reflect: a variable whose own address
	// escapes keeps its cells, so reflect sets its fields through &r (a
	// variable in bytes is read by reflect through a copy, docs/UNSAFE.md).
	var r struct {
		Count int
		Rate  float64
	}
	fmt.Sscan("7", &r.Count)
	rv := reflect.ValueOf(&r).Elem()
	rv.Field(1).SetFloat(2.5)
	fmt.Println(r, rv.Field(0).CanSet(), rv.Field(0).Int())
}
