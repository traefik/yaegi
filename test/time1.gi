package main

import (
	"time"
	"fmt"
)

func main() {
	t := time.Now()
	m := t.Minute()
	fmt.Println(t, m)
}
