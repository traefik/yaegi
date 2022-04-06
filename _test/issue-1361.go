package main

import (
	"fmt"
	"math"
)

type obj struct {
	num float64
}

type Fun func(o *obj) (r *obj, err error)

func numFun(fn func(f float64) float64) Fun {
	return func(o *obj) (*obj, error) {
		return &obj{fn(o.num)}, nil
	}
}

func main() {
	f := numFun(math.Cos)
	r, err := f(&obj{})
	fmt.Println(r, err)
}

// Output:
// &{1} <nil>
