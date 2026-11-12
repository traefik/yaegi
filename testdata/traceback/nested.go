// An unrecovered panic in nested calls: the traceback shows each frame at
// the line of the call (TestTraceback).
package main

func f(n int) {
	if n == 0 {
		var p *int
		println(*p)
	}
	f(n - 1)
}

func main() {
	f(2)
}
