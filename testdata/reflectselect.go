// reflect.Select on interpreted and native channels: send, receive,
// default, a closed channel, and the pattern of the traedis plugin (a send
// on a semaphore or ctx.Done()).
package main

import (
	"context"
	"fmt"
	"reflect"
	"time"
)

type item struct {
	Name string
	N    int
}

func acquire(ctx context.Context, sem chan struct{}) error {
	chosen, _, _ := reflect.Select([]reflect.SelectCase{
		{Dir: reflect.SelectSend, Chan: reflect.ValueOf(sem), Send: reflect.ValueOf(struct{}{})},
		{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(ctx.Done())},
	})
	if chosen == 1 {
		return ctx.Err()
	}
	return nil
}

func main() {
	sem := make(chan struct{}, 1)
	cases := []reflect.SelectCase{
		{Dir: reflect.SelectSend, Chan: reflect.ValueOf(sem), Send: reflect.ValueOf(struct{}{})},
		{Dir: reflect.SelectDefault},
	}
	chosen, _, _ := reflect.Select(cases)
	fmt.Println("first:", chosen, len(sem))
	chosen, _, _ = reflect.Select(cases)
	fmt.Println("second:", chosen, len(sem))

	done := make(chan int, 1)
	done <- 7
	chosen, v, ok := reflect.Select([]reflect.SelectCase{{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(done)}})
	fmt.Println("recv:", chosen, v.Int(), ok)

	// a struct sent by a goroutine, received by Select
	items := make(chan item)
	go func() { items <- item{"a", 1} }()
	chosen, v, ok = reflect.Select([]reflect.SelectCase{
		{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(make(chan int))},
		{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(items)},
	})
	got := v.Interface().(item)
	fmt.Println("struct:", chosen, got.Name, got.N, ok)

	// a closed channel: the zero value and ok false
	close(items)
	chosen, v, ok = reflect.Select([]reflect.SelectCase{{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(items)}})
	fmt.Println("closed:", chosen, v.Interface(), ok)

	// a send that a goroutine receives
	strs := make(chan string)
	res := make(chan string)
	go func() { res <- "got " + <-strs }()
	chosen, _, _ = reflect.Select([]reflect.SelectCase{
		{Dir: reflect.SelectSend, Chan: reflect.ValueOf(strs), Send: reflect.ValueOf("x")},
	})
	fmt.Println("send:", chosen, <-res)

	// a nil channel is never ready
	var nilc chan int
	chosen, _, _ = reflect.Select([]reflect.SelectCase{
		{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(nilc)},
		{Dir: reflect.SelectDefault},
	})
	fmt.Println("nil:", chosen)

	// traedis: the semaphore is full, the context ends the wait
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	fmt.Println("acquire:", acquire(ctx, sem))
	<-sem
	fmt.Println("acquire:", acquire(context.Background(), sem), len(sem))
}
