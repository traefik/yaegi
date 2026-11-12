package main

import (
	"fmt"
)

type Hello struct{}

func (*Hello) Hi() string {
	panic("implement me")
}

func main() {
	fmt.Println(&Hello{})
}

// Output:
// &{}
