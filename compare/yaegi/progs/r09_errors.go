package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
)

type NotFound struct{ Key string }

func (e *NotFound) Error() string { return "not found: " + e.Key }

type Temp struct{}

func (Temp) Error() string   { return "temp" }
func (Temp) Temporary() bool { return true }
func (Temp) Is(target error) bool { return target == ErrRetry }

var ErrRetry = errors.New("retry")

func main() {
	err := fmt.Errorf("layer2: %w", fmt.Errorf("layer1: %w", &NotFound{"k"}))
	var nf *NotFound
	fmt.Println(errors.As(err, &nf), nf.Key)
	fmt.Println(errors.Is(fmt.Errorf("x: %w", Temp{}), ErrRetry))
	var tmp interface{ Temporary() bool }
	fmt.Println(errors.As(fmt.Errorf("y: %w", Temp{}), &tmp))
	j := errors.Join(&NotFound{"a"}, ErrRetry)
	fmt.Println(errors.Is(j, ErrRetry), errors.As(j, &nf), nf.Key)
	_, err = os.Open("/nonexistent/x")
	var pe *fs.PathError
	fmt.Println(errors.As(err, &pe), errors.Is(err, fs.ErrNotExist))
}
