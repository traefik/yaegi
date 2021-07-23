// +build go1.16

package main

import (
	"errors"
	"io/fs"
)

func main() {
	pe := fs.PathError{}
	pe.Op = "nothing"
	pe.Path = "/nowhere"
	pe.Err = errors.New("an error")
	println(pe.Error())
}

// Output:
// nothing /nowhere: an error
