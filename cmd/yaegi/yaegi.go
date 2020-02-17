/*
Yaegi interprets Go programs.

Yaegi reads Go language programs from its standard input or from a file
and evaluates them.

If invoked with no arguments, it processes the standard input
in a Read-Eval-Print-Loop. A prompt is displayed if standard input
is a terminal.

Given a file, it operates on that file. If the first line starts with
"#!/usr/bin/env yaegi", and the file has exec permission, then the file
can be invoked directly from the shell.

In file mode, as in standard Go, files are read entirely, then parsed,
then evaluated. In REPL mode, each line is parsed and evaluated separately,
at global level in an implicit main package.

Options:
    -i
	   start an interactive REPL after file execution.
	-tags tag,list
	   a comma-separated list of build tags to consider satisfied during
	   the interpretation.

Debugging support (may be removed at any time):
  YAEGI_AST_DOT=1
    Generate and display graphviz dot of AST with dotty(1)
  YAEGI_CFG_DOT=1
    Generate and display graphviz dot of CFG with dotty(1)
*/
package main

import (
	"flag"
	"fmt"
	"go/build"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/containous/yaegi/interp"
	"github.com/containous/yaegi/stdlib"
)

func main() {
	var interactive bool
	var tags string
	var cmd string
	flag.BoolVar(&interactive, "i", false, "start an interactive REPL")
	flag.StringVar(&tags, "tags", "", "set a list of build tags")
	flag.StringVar(&cmd, "e", "", "set the command to be executed (instead of script or/and shell)")
	flag.Usage = func() {
		fmt.Println("Usage:", os.Args[0], "[options] [script] [args]")
		fmt.Println("Options:")
		flag.PrintDefaults()
	}
	flag.Parse()
	args := flag.Args()
	log.SetFlags(log.Lshortfile)

	i := interp.New(interp.Options{GoPath: build.Default.GOPATH, BuildTags: strings.Split(tags, ",")})
	i.Use(stdlib.Symbols)
	i.Use(interp.Symbols)

	if cmd != `` {
		i.REPL(strings.NewReader(cmd), os.Stderr)
	}

	if len(args) == 0 {
		if interactive || cmd == `` {
			i.REPL(os.Stdin, os.Stdout)
		}
		return
	}

	// Skip first os arg to set command line as expected by interpreted main
	os.Args = os.Args[1:]
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

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
		if p, ok := err.(interp.Panic); ok {
			fmt.Println(string(p.Stack))
		}
	}

	if interactive {
		i.REPL(os.Stdin, os.Stdout)
	}
}
