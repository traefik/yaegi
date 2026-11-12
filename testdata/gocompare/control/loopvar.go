// Go 1.22 per-iteration loop variables: closures and goroutines that capture
// the variable of a three-clause loop and of a range loop see their own
// iteration's value; changes inside the body are copied to the next
// iteration; pointers to the variable differ per iteration.
package main

import (
	"fmt"
	"sort"
	"sync"
)

func main() {
	var fs []func() int
	for i := 0; i < 5; i++ {
		fs = append(fs, func() int { return i })
	}
	for _, f := range fs {
		fmt.Print(f(), " ")
	}
	fmt.Println()

	var ps []*int
	for i := 0; i < 4; i++ {
		ps = append(ps, &i)
	}
	for _, p := range ps {
		fmt.Print(*p, " ")
	}
	fmt.Println(ps[0] != ps[1])

	fs = nil
	for _, v := range []int{10, 20, 30} {
		fs = append(fs, func() int { return v })
	}
	for _, f := range fs {
		fmt.Print(f(), " ")
	}
	fmt.Println()

	// The body changes the variable: the copy to the next iteration keeps it.
	fs = nil
	for i := 0; i < 10; i++ {
		fs = append(fs, func() int { return i })
		i += 2
	}
	for _, f := range fs {
		fmt.Print(f(), " ")
	}
	fmt.Println()

	// A closure that changes the variable in the same iteration.
	var incs []func()
	var reads []func() int
	for i := 0; i < 3; i++ {
		incs = append(incs, func() { i += 100 })
		reads = append(reads, func() int { return i })
	}
	incs[1]()
	incs[1]()
	for _, r := range reads {
		fmt.Print(r(), " ")
	}
	fmt.Println()

	// The post statement sees changes the closure made before it ran.
	var log []int
	for i := 0; i < 20; i++ {
		f := func() { i *= 2 }
		f()
		log = append(log, i)
	}
	fmt.Println(log)

	// Goroutines, collected in a deterministic order.
	var wg sync.WaitGroup
	var mu sync.Mutex
	var got []int
	for i := range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mu.Lock()
			got = append(got, i*i)
			mu.Unlock()
		}()
	}
	wg.Wait()
	sort.Ints(got)
	fmt.Println(got)

	res := make([]string, 4)
	var wg2 sync.WaitGroup
	for idx, s := range []string{"a", "b", "c", "d"} {
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			res[idx] = s + s
		}()
	}
	wg2.Wait()
	fmt.Println(res)

	ch := make(chan int, 5)
	for i := 0; i < 5; i++ {
		go func() { ch <- i * 10 }()
	}
	sum := 0
	for range 5 {
		sum += <-ch
	}
	fmt.Println("sum", sum)

	// Map range with captured key and value.
	m := map[string]int{"x": 1, "y": 2, "z": 3}
	var mfs []func() string
	for k, v := range m {
		mfs = append(mfs, func() string { return fmt.Sprint(k, v) })
	}
	var outs []string
	for _, f := range mfs {
		outs = append(outs, f())
	}
	sort.Strings(outs)
	fmt.Println(outs)

	// Nested loops capture both variables.
	var pairs []func() [2]int
	for i := 0; i < 2; i++ {
		for j := 0; j < 2; j++ {
			pairs = append(pairs, func() [2]int { return [2]int{i, j} })
		}
	}
	for _, p := range pairs {
		fmt.Print(p(), " ")
	}
	fmt.Println()

	// defer in a loop captures the iteration's variable.
	func() {
		for i := 0; i < 3; i++ {
			defer func() { fmt.Print("d", i, " ") }()
		}
	}()
	fmt.Println()

	// String range: index and rune captured.
	var sfs []func() string
	for i, r := range "héllo" {
		sfs = append(sfs, func() string { return fmt.Sprintf("%d:%c", i, r) })
	}
	for _, f := range sfs {
		fmt.Print(f(), " ")
	}
	fmt.Println()
}
