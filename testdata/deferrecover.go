package main

import "fmt"

// "defer recover()" acts like a recover call in the function that deferred
// it, when that function returns. When a newer panic of the same function was
// recovered by an earlier deferred call, the function returns normally, so
// the deferred recover() stops the older panic (GOROOT test recover1.go,
// test6). While the newer panic is still active, it does nothing (test7).

// The newer panic 3 is recovered first, then "defer recover()" stops panic 2.
func newerRecovered() {
	defer func() { fmt.Println("newerRecovered, outer:", recover()) }()
	defer func() {
		defer recover()
		defer func() { fmt.Println("newerRecovered, inner:", recover()) }()
		panic(3)
	}()
	panic(2)
}

// Same, but the newer panic is recovered two calls earlier.
func newerRecoveredEarlier() {
	defer func() { fmt.Println("newerRecoveredEarlier, outer:", recover()) }()
	defer func() {
		defer recover()
		defer fmt.Println("newerRecoveredEarlier, between")
		defer func() { fmt.Println("newerRecoveredEarlier, inner:", recover()) }()
		panic(3)
	}()
	panic(2)
}

// The newer panic is still active when "defer recover()" runs: it does
// nothing, and each panic is recovered by its own deferred call.
func newerActive() {
	defer func() { fmt.Println("newerActive, outer:", recover()) }()
	defer func() {
		defer func() { fmt.Println("newerActive, inner:", recover()) }()
		defer recover()
		panic(3)
	}()
	panic(2)
}

// A function that is not run by a panic: "defer recover()" sees nothing, even
// after a recovered panic of its own.
func notDeferred() {
	defer func() { fmt.Println("notDeferred, outer:", recover()) }()
	func() {
		defer recover()
		defer func() { fmt.Println("notDeferred, inner:", recover()) }()
		panic(3)
	}()
	fmt.Println("notDeferred: returns")
}

func main() {
	newerRecovered()
	newerRecoveredEarlier()
	newerActive()
	notDeferred()
}
