// A channel made by the program goes to native code as a real channel when
// its element is struct{} or bool: context.WithCancel waits on the Done
// channel of a Context of the program, and a closed Done cancels the
// child. Before, native code got a closed stand-in, or the program
// stopped with "passing interpreted channel to native code".
package main

import (
	"context"
	"fmt"
	"reflect"
	"time"
)

type myCtx struct {
	context.Context
	done chan struct{}
}

func (c myCtx) Done() <-chan struct{} { return c.done }

func (c myCtx) Err() error {
	select {
	case <-c.done:
		return context.Canceled
	default:
		return nil
	}
}

func main() {
	parent := myCtx{context.Background(), make(chan struct{})}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	select {
	case <-ctx.Done():
		fmt.Println("canceled too early")
	case <-time.After(10 * time.Millisecond):
		fmt.Println("not canceled yet")
	}
	close(parent.done)
	select {
	case <-ctx.Done():
		fmt.Println("child canceled:", ctx.Err())
	case <-time.After(5 * time.Second):
		fmt.Println("child not canceled")
	}

	// an alias of struct{} is struct{}: a native channel too
	type E = struct{}
	ae := make(chan E)
	actx, acancel := context.WithCancel(myCtx{context.Background(), ae})
	defer acancel()
	close(ae)
	<-actx.Done()
	fmt.Println("alias child canceled:", actx.Err())

	// the channels keep their behavior in the program
	b := make(chan bool, 2)
	b <- true
	b <- false
	fmt.Println(len(b), cap(b), <-b, <-b)
	close(b)
	v, ok := <-b
	fmt.Println(v, ok)
	s := make(chan struct{})
	go func() { defer close(s) }()
	_, ok = <-s
	fmt.Println(ok, s != nil, reflect.TypeOf(s))
	m := map[chan struct{}]int{s: 1}
	fmt.Println(m[s])
	var nilc chan struct{}
	select {
	case <-nilc:
	default:
		fmt.Println("nil chan")
	}
	func() {
		defer func() { fmt.Println("close nil:", recover()) }()
		defer close(nilc)
	}()
	func() {
		defer func() { fmt.Println("make negative:", recover()) }()
		n := -1
		_ = make(chan bool, n)
	}()
}
