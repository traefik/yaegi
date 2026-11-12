package main

import (
	"fmt"
)

type Cheese struct {
	property string
}

func (t *Cheese) Hello(param string) {
	fmt.Printf("%+v %+v", t, param)
}

func main() {
	(*Cheese).Hello(&Cheese{property: "value"}, "param")
}

// Output:
// &{Xproperty:value} param
