// An unrecovered panic in a goroutine, after a recovered one: the
// traceback shows the goroutine's frames (TestTraceback).
package main

func work(c chan int) {
	defer func() { recover() }()
	var m map[string]int
	m["x"] = 1 // recovered
}

func crash(c chan int) {
	work(c)
	panic("in goroutine")
}

func main() {
	c := make(chan int)
	go crash(c)
	<-c
}
