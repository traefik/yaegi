package main

import (
	"fmt"
	"time"
)

type Options struct {
	debug bool
}

type T1 struct {
	opt  Options
	time time.Time
}

func main() {
	t := T1{}
	t.time = time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
	fmt.Println(t.time)
}

// Output:
// 2009-11-10 23:00:00 +0000 UTC
