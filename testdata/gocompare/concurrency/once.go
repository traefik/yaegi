// sync.Once, OnceFunc, OnceValue and OnceValues: called from many goroutines
// the function runs once, a panic in OnceFunc is re-raised at each call, and
// a panic in Once.Do counts as done.
package main

import (
	"errors"
	"fmt"
	"sync"
)

func main() {
	var once sync.Once
	var wg sync.WaitGroup
	calls := 0
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			once.Do(func() { calls++ })
		}()
	}
	wg.Wait()
	fmt.Println("calls", calls)
	once.Do(func() { calls += 100 })
	fmt.Println("calls after", calls)

	// Panic in Do: the Once is done anyway.
	var po sync.Once
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		po.Do(func() { panic("boom in Do") })
	}()
	ran := false
	po.Do(func() { ran = true })
	fmt.Println("ran after panic", ran)

	// Nested Once values.
	var o1, o2 sync.Once
	order := []string{}
	o1.Do(func() {
		order = append(order, "o1")
		o2.Do(func() { order = append(order, "o2") })
	})
	o2.Do(func() { order = append(order, "o2 again") })
	fmt.Println(order)

	// OnceFunc.
	n := 0
	f := sync.OnceFunc(func() { n++ })
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f()
		}()
	}
	wg.Wait()
	f()
	fmt.Println("OnceFunc n", n)

	pf := sync.OnceFunc(func() { panic("once func panic") })
	for i := 0; i < 3; i++ {
		func() {
			defer func() { fmt.Println("pf", i, recover()) }()
			pf()
		}()
	}

	// OnceValue.
	computed := 0
	v := sync.OnceValue(func() int {
		computed++
		s := 0
		for i := 1; i <= 100; i++ {
			s += i
		}
		return s
	})
	results := make([]int, 8)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = v()
		}()
	}
	wg.Wait()
	fmt.Println(results, computed)

	sv := sync.OnceValue(func() string { return "hello" })
	fmt.Println(sv(), sv())

	pv := sync.OnceValue(func() int { panic(errors.New("value panic")) })
	for i := 0; i < 2; i++ {
		func() {
			defer func() {
				r := recover()
				err, ok := r.(error)
				fmt.Println("pv", i, ok, err)
			}()
			pv()
		}()
	}

	// OnceValues.
	cnt := 0
	load := sync.OnceValues(func() (map[string]int, error) {
		cnt++
		return map[string]int{"a": 1, "b": 2}, nil
	})
	m1, err1 := load()
	m2, err2 := load()
	fmt.Println(m1, err1, m2, err2, cnt)
	m1["c"] = 3
	m3, _ := load()
	fmt.Println("shared map", m3)

	fail := sync.OnceValues(func() (int, error) {
		cnt++
		return 0, errors.New("load failed")
	})
	for i := 0; i < 3; i++ {
		x, err := fail()
		fmt.Println(x, err)
	}
	fmt.Println("cnt", cnt)
}
