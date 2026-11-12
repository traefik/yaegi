package main

import (
	"fmt"

	"github.com/cespare/xxhash/v2"
)

func main() {
	fmt.Println(xxhash.Sum64String("hello world"))
	fmt.Println(xxhash.Sum64([]byte("hello world")))
	d := xxhash.New()
	d.WriteString("hello ")
	d.Write([]byte("world"))
	fmt.Println(d.Sum64())
	big := make([]byte, 1000)
	for i := range big {
		big[i] = byte(i)
	}
	fmt.Println(xxhash.Sum64(big))
}
