package main

import "fmt"

type Mover interface{ Move() error }

type car struct{ Mover }

type engine struct{}

func (e *engine) Move() error { fmt.Println("engine moved"); return nil }

func newMover() Mover { return car{Mover: &engine{}} }

func main() {
	mover := newMover()
	if _, ok := mover.(car); ok {
		fmt.Println("mover is a car")
	}
	mover.Move()
}

// Output:
// mover is a car
// engine moved
