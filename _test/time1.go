package main

import (
	"fmt"
	"time"
)

func main() {
	t := time.Now()
	m := t.Minute()
	fmt.Println(t, m)
}
