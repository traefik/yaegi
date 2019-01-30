package main

import (
	"fmt"

	fr "github.com/foo/pkg/fromage"
)

func Here() string {
	return "root"
}

func main() {
	//a := fmt.Sprintf("%s %s", Here(), fromage.Hello())
	a := fmt.Sprintf("%s %s", Here(), fr.Hello())
	println(a)
}
