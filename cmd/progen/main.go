// Command progen prints the random Go program of a seed (internal/progen),
// to look at a program that TestGeneratedPrograms of cmd/runtests reports:
//
//	go run ./cmd/progen -seed 42 > /tmp/p.go && go run /tmp/p.go && go run ./cmd/yaegi /tmp/p.go
package main

import (
	"flag"
	"fmt"

	"github.com/traefik/yaegi/v2/internal/progen"
)

func main() {
	seed := flag.Uint64("seed", 0, "seed of the program")
	flag.Parse()
	fmt.Print(progen.Generate(*seed))
}
