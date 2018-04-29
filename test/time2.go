// +build ignore
package main

import (
	"time"
	"fmt"
)

func main() {
	t := time.Now()
	h, m, s := t.Clock()
	fmt.Println(h, m, s)
}
