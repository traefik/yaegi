package main

import (
	"time"
)

type foo struct {
	bar string
}

func main() {
	for i := 0; i < 2; i++ {
		go func() {
			a := foo{bar: "hello"}
			println(a)
		}()
	}
	time.Sleep(time.Second)
}
