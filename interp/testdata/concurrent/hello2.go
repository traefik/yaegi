package main

import "time"

func main() {
	go func() {
		time.Sleep(3 * time.Second)
		println("hello world2")
	}()
}
