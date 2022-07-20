package fs1

import (
	"testing"
	"testing/fstest" // only available from 1.16.

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
)

var testFilesystem = fstest.MapFS{
	"main.go": &fstest.MapFile{
		Data: []byte(`package main

import (
	"foo/bar"
	"./localfoo"
)

func main() {
	bar.PrintSomething()
	localfoo.PrintSomethingElse()
}
`),
	},
	"_pkg/src/foo/bar/bar.go": &fstest.MapFile{
		Data: []byte(`package bar

import (
	"fmt"
)

func PrintSomething() {
	fmt.Println("I am a virtual filesystem printing something from _pkg/src/foo/bar/bar.go!")
}
`),
	},
	"localfoo/foo.go": &fstest.MapFile{
		Data: []byte(`package localfoo

import (
	"fmt"
)

func PrintSomethingElse() {
	fmt.Println("I am virtual filesystem printing else from localfoo/foo.go!")
}
`),
	},
}

func TestFilesystemMapFS(t *testing.T) {
	i := interp.New(interp.Options{
		GoPath:               "./_pkg",
		SourcecodeFilesystem: testFilesystem,
	})
	if err := i.Use(stdlib.Symbols); err != nil {
		t.Fatal(err)
	}

	_, err := i.EvalPath(`main.go`)
	if err != nil {
		t.Fatal(err)
	}
}
