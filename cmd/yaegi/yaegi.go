package main

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
	var interactive bool
	flag.BoolVar(&interactive, "i", false, "start an interactive REPL")
	flag.Usage = func() {
		fmt.Println("Usage:", os.Args[0], "[options] [script] [args]")
		fmt.Println("Options:")
		flag.PrintDefaults()
	}
	flag.Parse()
	args := flag.Args()
	log.SetFlags(log.Lshortfile)

	i := interp.New(interp.Options{})
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

		if interactive {
			i.Repl(os.Stdin, os.Stdout)
		}
	} else {
		i.Repl(os.Stdin, os.Stdout)
	}
}
