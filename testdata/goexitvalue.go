package main

// runtime.Goexit as a func value, run by native code on a goroutine with no
// VM frame: it ends that goroutine quietly, and the program goes on.

import (
	"fmt"
	"runtime"
	"sync"
	"time"
)

func main() {
	f := runtime.Goexit
	time.AfterFunc(0, f)
	var o sync.Once
	go o.Do(f) // a native method that calls the value
	time.Sleep(100 * time.Millisecond)
	fmt.Println("alive")
}
