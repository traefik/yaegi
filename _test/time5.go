package main

import (
	"fmt"
	"time"
)

func main() {
	//	t := time.Now()
	t := time.Unix(1000000000, 0)
	m := t.Minute()
	fmt.Println(t, m)
}
