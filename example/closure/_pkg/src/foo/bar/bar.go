package bar

import (
	"fmt"
)

var version = "v1"

func NewSample() func(string, string) func(string) string {
	fmt.Println("in NewSample")
	return func(val string, name string) func(string) string {
		fmt.Println("in function", version, val, name)
		return func(msg string) string {
			return fmt.Sprint("here", version, val, name, msg)
		}
	}
}
