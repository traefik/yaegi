package main

//go:generate go generate github.com/containous/yaegi/interp
//go:generate go generate github.com/containous/yaegi/cmd/goexports
//go:generate go generate github.com/containous/yaegi/stdlib
//go:generate go generate github.com/containous/yaegi/stdlib/syscall
//go:generate go generate github.com/containous/yaegi/stdlib/unsafe

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/containous/yaegi/interp"
	"github.com/containous/yaegi/stdlib"
)

func main() {
	flag.Usage = func() { fmt.Println("Usage:", os.Args[0], "[script] [args]") }
	flag.Parse()
	args := flag.Args()

	i := interp.New()
	i.Use(stdlib.Symbols)
	i.Use(interp.Symbols)

	if len(args) > 0 {
		// Skip first os arg to set command line as expected by interpreted main
		os.Args = os.Args[1:]
		b, err := ioutil.ReadFile(args[0])
		if err != nil {
			log.Fatal("Could not read file: ", args[0])
		}
		s := string(b)
		if s[:2] == "#!" {
			// Allow executable go scripts, but fix them prior to parse
			s = strings.Replace(s, "#!", "//", 1)
		}
		i.Name = args[0]
		if _, err := i.Eval(s); err != nil {
			fmt.Println(err)
		}
	} else {
		i.Repl(os.Stdin, os.Stdout)
	}
}
