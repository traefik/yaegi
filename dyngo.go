package main

//go:generate go generate github.com/containous/dyngo/stdlib

import (
	"bufio"
	"flag"
	"fmt"
	"go/scanner"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/containous/dyngo/interp"
	"github.com/containous/dyngo/stdlib"
)

func main() {
	opt := interp.Opt{Entry: "main"}
	var interactive bool
	flag.BoolVar(&opt.AstDot, "a", false, "display AST graph")
	flag.BoolVar(&opt.CfgDot, "c", false, "display CFG graph")
	flag.BoolVar(&interactive, "i", false, "start an interactive REPL")
	flag.BoolVar(&opt.NoRun, "n", false, "do not run")
	flag.Usage = func() {
		fmt.Println("Usage:", os.Args[0], "[options] [script] [args]")
		fmt.Println("Options:")
		flag.PrintDefaults()
	}
	flag.Parse()
	args := flag.Args()
	log.SetFlags(log.Lshortfile)
	if len(args) > 0 {
		b, err := ioutil.ReadFile(args[0])
		if err != nil {
			log.Fatal("Could not read file: ", args[0])
		}
		s := string(b)
		if s[:2] == "#!" {
			// Allow executable go scripts, but fix them prior to parse
			s = strings.Replace(s, "#!", "//", 1)
		}
		i := interp.New(opt)
		i.Use(stdlib.Value, stdlib.Type)
		i.Use(interp.ExportValue, interp.ExportType)
		if _, err := i.Eval(string(s)); err != nil {
			fmt.Println(err)
		}
		if interactive {
			repl(i)
		}
	} else {
		i := interp.New(opt)
		i.Use(stdlib.Value, stdlib.Type)
		i.Use(interp.ExportValue, interp.ExportType)
		repl(i)
	}
}

func repl(i *interp.Interpreter) {
	s := bufio.NewScanner(os.Stdin)
	prompt := getPrompt()
	prompt()
	src := ""
	for s.Scan() {
		src += s.Text() + "\n"
		if v, err := i.Eval(src); err != nil {
			switch err.(type) {
			case scanner.ErrorList:
				continue
			}
			fmt.Println(err)
		} else if v.IsValid() {
			fmt.Println(v)
		}
		src = ""
		prompt()
	}
}

// getPrompt returns a function which prints a prompt only if stdin is a terminal
func getPrompt() func() {
	if stat, err := os.Stdin.Stat(); err == nil && stat.Mode()&os.ModeCharDevice != 0 {
		return func() { fmt.Print("> ") }
	}
	return func() {}
}
