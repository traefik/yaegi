// Panics inside goroutines, recovered in the goroutine and reported through
// a channel: runtime errors (index, nil map, nil pointer, division by zero,
// type assertion), custom errors, re-panics, and a safe-go helper.
package main

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
)

type result struct {
	idx int
	val int
	err string
}

func safeGo(wg *sync.WaitGroup, out []result, idx int, f func() int) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				_, rt := r.(runtime.Error)
				out[idx] = result{idx: idx, err: fmt.Sprintf("%v (runtime=%v)", r, rt)}
			}
		}()
		out[idx] = result{idx: idx, val: f()}
	}()
}

type myErr struct{ msg string }

func (e myErr) Error() string { return "myErr: " + e.msg }

func main() {
	var wg sync.WaitGroup
	tasks := []func() int{
		func() int { return 42 },
		func() int { s := []int{1, 2}; i := 5; return s[i] },
		func() int { var m map[string]int; m["x"] = 1; return 0 },
		func() int { var p *struct{ x int }; return p.x },
		func() int { a, b := 10, 0; return a / b },
		func() int { var i interface{} = "s"; return i.(int) },
		func() int { panic(myErr{"custom"}) },
		func() int { panic(fmt.Sprintf("formatted %d", 7)) },
		func() int { panic(errors.New("plain error")) },
		func() int { s := make([]int, 3); j := 1; return len(s[2:j]) },
		func() int { var f func() int; return f() },
		func() int { a := []int{1}; return [1]int(a[:0])[0] },
	}
	out := make([]result, len(tasks))
	for i, f := range tasks {
		safeGo(&wg, out, i, f)
	}
	wg.Wait()
	for _, r := range out {
		fmt.Printf("%d: val=%d err=%s\n", r.idx, r.val, r.err)
	}

	// Report through a channel: the panic value and a recovered error.
	errc := make(chan error)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				if e, ok := r.(error); ok {
					errc <- fmt.Errorf("goroutine failed: %w", e)
					return
				}
				errc <- fmt.Errorf("goroutine failed: %v", r)
			}
		}()
		panic(myErr{"via channel"})
	}()
	err := <-errc
	var me myErr
	fmt.Println(err, errors.As(err, &me), me.msg)

	// Re-panic in a deferred func, the outer recover sees the last value.
	res := make(chan string)
	go func() {
		defer func() { res <- fmt.Sprint("outer: ", recover()) }()
		defer func() {
			r := recover()
			panic(fmt.Sprint("re-panic of ", r))
		}()
		panic("first")
	}()
	fmt.Println(<-res)

	// Deferred funcs run in order before the recover in the goroutine.
	logc := make(chan []string)
	go func() {
		var log []string
		defer func() {
			r := recover()
			log = append(log, fmt.Sprint("recover ", r))
			logc <- log
		}()
		defer func() { log = append(log, "defer 2") }()
		defer func() { log = append(log, "defer 1") }()
		log = append(log, "body")
		panic("stop")
	}()
	fmt.Println(<-logc)

	// panic(nil) is a *runtime.PanicNilError.
	pn := make(chan string)
	go func() {
		defer func() {
			r := recover()
			_, isPN := r.(*runtime.PanicNilError)
			pn <- fmt.Sprint(r, " ", isPN)
		}()
		panic(nil)
	}()
	fmt.Println(<-pn)

	// Many goroutines, half panic: count by kind.
	var mu sync.Mutex
	panics, oks := 0, 0
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				mu.Lock()
				defer mu.Unlock()
				if recover() != nil {
					panics++
				} else {
					oks++
				}
			}()
			if i%2 == 1 {
				panic(i)
			}
		}()
	}
	wg.Wait()
	fmt.Println("panics", panics, "oks", oks)
}
