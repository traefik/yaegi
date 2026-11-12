package main

import (
	"context"
	"fmt"
	"time"
)

// Channels created by native code, used by interpreted code.

func main() {
	for t := range time.Tick(time.Millisecond) {
		fmt.Println("tick", !t.IsZero())
		break
	}

	t := time.NewTimer(time.Millisecond)
	v, ok := <-t.C
	fmt.Println(!v.IsZero(), ok, cap(t.C), len(t.C))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	select {
	case x, ok := <-ctx.Done():
		fmt.Println("done", x, ok)
	case <-time.After(time.Second):
		fmt.Println("timeout")
	}

	ic := make(chan int, 3)
	ic <- 1
	fmt.Println(len(ic), cap(ic))
}
