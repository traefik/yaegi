package main

import (
	"fmt"
	"time"
)

const (
	period    = 100 * time.Millisecond
	precision = 5 * time.Millisecond
)

func main() {
	counter := 0
	p := time.Now()
	ticker := time.NewTicker(period)
	ch := make(chan int)

	go func() {
		for i := 0; i < 3; i++ {
			select {
			case t := <-ticker.C:
				counter = counter + 1
				ch <- counter
				if d := t.Sub(p) - period; d < -precision || d > precision {
					fmt.Println("wrong delay", d)
				}
				p = t
			}
		}
		ch <- 0
	}()
	for c := range ch {
		if c == 0 {
			break
		}
		println(c)
	}
}

// Output:
// 1
// 2
// 3
