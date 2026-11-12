// context.AfterFunc: the func runs once after cancel, stop before cancel
// prevents it, stop after it ran returns false, AfterFunc on a canceled
// context runs at once; also a custom Context type used as a parent. The
// program waits on channels, never on time.
package main

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
)

func main() {
	bg := context.Background()

	ctx, cancel := context.WithCancel(bg)
	ran := make(chan string, 1)
	stop := context.AfterFunc(ctx, func() { ran <- "after func ran" })
	cancel()
	fmt.Println(<-ran)
	fmt.Println("stop after run", stop())

	// Stop before cancel: the func does not run.
	ctx2, cancel2 := context.WithCancel(bg)
	var count atomic.Int32
	stop2 := context.AfterFunc(ctx2, func() { count.Add(1) })
	fmt.Println("stop before", stop2(), stop2())
	cancel2()
	// One more AfterFunc to know the cancel work is over.
	sync2 := make(chan struct{})
	context.AfterFunc(ctx2, func() { close(sync2) })
	<-sync2
	fmt.Println("count", count.Load())

	// Several funcs on the same context: all run once.
	ctx3, cancel3 := context.WithCancel(bg)
	var wg sync.WaitGroup
	results := make([]int, 5)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		context.AfterFunc(ctx3, func() {
			defer wg.Done()
			results[i] = i * 10
		})
	}
	cancel3()
	cancel3()
	wg.Wait()
	fmt.Println(results)

	// On an already canceled context: runs at once (in its goroutine).
	done := make(chan bool)
	stop4 := context.AfterFunc(ctx3, func() { done <- true })
	fmt.Println("canceled ctx", <-done, stop4())

	// AfterFunc on a child: canceled by the parent.
	parent, pcancel := context.WithCancelCause(bg)
	child := context.WithValue(parent, "k", "v")
	got := make(chan string)
	context.AfterFunc(child, func() {
		got <- fmt.Sprint(child.Value("k"), " ", context.Cause(child))
	})
	pcancel(fmt.Errorf("shutdown"))
	fmt.Println(<-got)

	// Parent that is never canceled: stop returns true.
	stop5 := context.AfterFunc(bg, func() { fmt.Println("never") })
	fmt.Println("background stop", stop5())

	// A context that is closed by AfterFunc of another one (linking).
	src, srcCancel := context.WithCancel(bg)
	dst, dstCancel := context.WithCancelCause(bg)
	context.AfterFunc(src, func() { dstCancel(fmt.Errorf("linked")) })
	srcCancel()
	<-dst.Done()
	fmt.Println("dst", dst.Err(), context.Cause(dst))
}
