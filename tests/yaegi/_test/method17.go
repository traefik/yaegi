package main

import (
	"fmt"
	"time"
)

type MyTime struct {
	time.Time
	index int
}

func (m MyTime) Foo() {
	minute := m.Minute()
	fmt.Println("minute:", minute)
}

func (m *MyTime) Bar() {
	second := m.Second()
	fmt.Println("second:", second)
}

func main() {
	t := MyTime{}
	t.Time = time.Date(2009, time.November, 10, 23, 4, 5, 0, time.UTC)
	t.Foo()
	t.Bar()
	(&t).Bar()
}

// Output:
// minute: 4
// second: 5
// second: 5
