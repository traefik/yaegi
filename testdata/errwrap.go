package main

import (
	"errors"
	"fmt"
)

type notFound struct{ name string }

func (e *notFound) Error() string { return "not found: " + e.name }

var errBase = errors.New("base")

func main() {
	wrapped := fmt.Errorf("op failed: %w", errBase)
	fmt.Println(errors.Is(wrapped, errBase))

	nf := &notFound{"key"}
	err := fmt.Errorf("lookup: %w", nf)
	var target *notFound
	fmt.Println(errors.As(err, &target), target.name)

	joined := errors.Join(errBase, nf)
	fmt.Println(errors.Is(joined, errBase))
	fmt.Println(errors.As(joined, &target))
	fmt.Println(joined)
}
