// Channels: len and cap of nil, unbuffered and buffered channels, values
// kept in FIFO order, receive from a closed channel (buffered values first,
// then the zero value with ok=false), and comparison of channel values.
package main

import "fmt"

type pair struct {
	a int
	b string
}

func main() {
	var nilc chan int
	fmt.Println("nil:", nilc == nil, len(nilc), cap(nilc))

	u := make(chan int)
	fmt.Println("unbuffered:", len(u), cap(u), u != nil)

	b := make(chan int, 5)
	for i := 1; i <= 3; i++ {
		b <- i * 10
		fmt.Println("after send", i, "len", len(b), "cap", cap(b))
	}
	v := <-b
	fmt.Println("recv", v, "len", len(b))
	b <- 40
	b <- 50
	b <- 60
	fmt.Println("full len", len(b), "cap", cap(b))
	close(b)
	fmt.Println("closed len", len(b))
	for i := 0; i < 7; i++ {
		x, ok := <-b
		fmt.Println("recv", i, x, ok, len(b))
	}

	// Zero values of several element types after close.
	cs := make(chan string, 1)
	cs <- "hello"
	close(cs)
	s1, ok1 := <-cs
	s2, ok2 := <-cs
	fmt.Printf("%q %v %q %v\n", s1, ok1, s2, ok2)

	cp := make(chan pair, 1)
	cp <- pair{1, "x"}
	close(cp)
	p1, okp1 := <-cp
	fmt.Printf("%+v %v\n", p1, okp1)

	cptr := make(chan *int)
	close(cptr)
	pp, okpp := <-cptr
	fmt.Println(pp == nil, okpp)

	ci := make(chan interface{}, 2)
	ci <- 3
	ci <- "s"
	close(ci)
	for x := range ci {
		fmt.Printf("%T %v\n", x, x)
	}
	z, okz := <-ci
	fmt.Println(z, okz)

	cf := make(chan float64)
	close(cf)
	fmt.Println(<-cf)

	ca := make(chan [3]int, 1)
	arr := [3]int{1, 2, 3}
	ca <- arr
	arr[0] = 99
	fmt.Println(<-ca, arr)

	// Channel identity.
	c1 := make(chan int)
	c2 := c1
	c3 := make(chan int)
	fmt.Println(c1 == c2, c1 == c3)
	m := map[chan int]string{c1: "one", c3: "three"}
	fmt.Println(m[c2], m[c3], len(m))

	// Unbuffered handshake with a goroutine.
	done := make(chan bool)
	data := make(chan int)
	go func() {
		sum := 0
		for x := range data {
			sum += x
		}
		fmt.Println("goroutine sum", sum)
		done <- true
	}()
	for i := 1; i <= 100; i++ {
		data <- i
	}
	close(data)
	<-done
	fmt.Println("end")
}
