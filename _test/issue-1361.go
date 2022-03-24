package main

import (
	"fmt"
	"math"
)

type obj struct {
	num float64
}

type Func func(o *obj) (r *obj, err error)

func numFunc(fn func(f float64) float64) Func {
	return func(o *obj) (*obj, error) {
		return &obj{fn(o.num)}, nil
	}
}

func main() {
	f := numFunc(math.Cos)
	r, err := f(&obj{})
	fmt.Println(r, err)
}

// Output:
// &{1} <nil>
