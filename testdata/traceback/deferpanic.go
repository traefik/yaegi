// A deferred call panics during a panic: the traceback shows the stack of
// the second panic, with the frames that the first one left
// (TestTraceback).
package main

func g() {
	defer func() {
		panic("second")
	}()
	panic("first")
}

func main() {
	g()
}
