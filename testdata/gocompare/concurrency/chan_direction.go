// Directional channel types: conversion of a bidirectional channel to
// send-only and receive-only, %T of each, functions that take directional
// parameters, and channel types in type switches.
package main

import "fmt"

func producer(out chan<- int, n int) {
	for i := 0; i < n; i++ {
		out <- i * 3
	}
	close(out)
}

func consumer(in <-chan int, done chan<- int) {
	s := 0
	for v := range in {
		s += v
	}
	done <- s
}

func kind(x interface{}) string {
	switch x.(type) {
	case chan int:
		return "chan int"
	case <-chan int:
		return "<-chan int"
	case chan<- int:
		return "chan<- int"
	case chan string:
		return "chan string"
	default:
		return "other"
	}
}

type Sender chan<- string
type Receiver <-chan string

func main() {
	c := make(chan int, 4)
	var s chan<- int = c
	var r <-chan int = c
	fmt.Printf("%T %T %T\n", c, s, r)
	fmt.Println(kind(c), kind(s), kind(r), kind(make(chan string)), kind(3))
	s <- 1
	s <- 2
	fmt.Println(len(r), cap(s), len(c))
	fmt.Println(<-r, <-c)

	// Conversion expressions.
	r2 := (<-chan int)(c)
	s2 := (chan<- int)(c)
	s2 <- 9
	fmt.Println(<-r2)

	done := make(chan int)
	pc := make(chan int)
	go producer(pc, 10)
	go consumer(pc, done)
	fmt.Println("sum", <-done)

	// Named directional types.
	bc := make(chan string, 2)
	var snd Sender = bc
	var rcv Receiver = bc
	snd <- "a"
	snd <- "b"
	fmt.Printf("%T %T %s %s\n", snd, rcv, <-rcv, <-rcv)

	// Channel of directional channels.
	cc := make(chan (<-chan int), 1)
	inner := make(chan int, 1)
	inner <- 42
	cc <- inner
	fmt.Printf("%T %d\n", cc, <-<-cc)

	// chan <-chan int parses as chan<- (chan int).
	ss := make(chan<- chan int, 1)
	ss <- inner
	fmt.Printf("%T %d\n", ss, len(ss))

	// Func returning a receive-only channel.
	f := func() <-chan string {
		ch := make(chan string, 1)
		ch <- "from func"
		close(ch)
		return ch
	}
	for v := range f() {
		fmt.Println(v)
	}

	// Assigning a directional channel to interface and back.
	var any interface{} = r
	if rr, ok := any.(<-chan int); ok {
		fmt.Println("recv-only assert ok", len(rr))
	}
	_, ok := any.(chan int)
	fmt.Println("bidir assert", ok)
}
