// Timers and tickers that never fire: an hour-long timer stopped at once
// (Stop returns true then false), Reset of a stopped timer, time.AfterFunc
// stopped before it runs, a ticker stopped, and a select on a timer channel
// that is not ready. Nothing waits on the clock.
package main

import (
	"fmt"
	"time"
)

func main() {
	t := time.NewTimer(time.Hour)
	fmt.Println("stop", t.Stop(), t.Stop())
	select {
	case <-t.C:
		fmt.Println("bad: fired")
	default:
		fmt.Println("not fired")
	}
	fmt.Println("reset stopped", t.Reset(2*time.Hour))
	fmt.Println("reset active", t.Reset(3*time.Hour))
	fmt.Println("stop again", t.Stop())
	fmt.Println(len(t.C), cap(t.C))

	ran := false
	af := time.AfterFunc(time.Hour, func() { ran = true })
	fmt.Println("afterfunc stop", af.Stop(), af.Stop(), ran)
	fmt.Println(af.C == nil)

	tk := time.NewTicker(time.Hour)
	select {
	case <-tk.C:
		fmt.Println("bad tick")
	default:
		fmt.Println("no tick")
	}
	tk.Reset(2 * time.Hour)
	tk.Stop()
	tk.Stop()

	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		time.NewTicker(0)
	}()
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		time.NewTicker(time.Second).Reset(-1)
	}()

	// A select with a long timer and a ready channel: the channel wins.
	ready := make(chan string, 1)
	ready <- "value"
	timer := time.NewTimer(time.Hour)
	select {
	case v := <-ready:
		fmt.Println("got", v)
	case <-timer.C:
		fmt.Println("bad timeout")
	}
	fmt.Println(timer.Stop())

	// time.After in a select that is decided by a closed channel.
	closed := make(chan struct{})
	close(closed)
	for i := 0; i < 3; i++ {
		select {
		case <-closed:
			fmt.Println("closed wins", i)
		case <-time.After(time.Hour):
			fmt.Println("bad")
		}
	}
}
