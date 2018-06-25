package main

import (
	"fmt"
	"time"
)

func main() {
	t := time.Now()
	h, m, s := t.Clock()
	fmt.Println(h, m, s)
}
