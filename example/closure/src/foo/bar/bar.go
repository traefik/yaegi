package bar

import (
	"fmt"
)

var version = "v1"

func NewSample() func(string, string) func(string) {
	fmt.Println("in NewSample")
	return func(val string, name string) func(string) {
		fmt.Println("in function", version, val, name)
		return func(msg string) {
			fmt.Println("here", version, val, name, msg)
		}
	}
}
