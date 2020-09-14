package main

import (
	"fmt"
	"math/rand"
	"testing"
)

func TestMain(t *testing.T) {
	fmt.Println("in test")
}

func BenchmarkMain(b *testing.B) {
	for i := 0; i < b.N; i++ {
		rand.Int()
	}
}
