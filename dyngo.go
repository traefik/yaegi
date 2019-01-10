package main

//go:generate go generate github.com/containous/dyngo/cmd/goexports
//go:generate go generate github.com/containous/dyngo/stdlib
//go:generate go generate github.com/containous/dyngo/stdlib/syscall

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/containous/dyngo/interp"
	"github.com/containous/dyngo/stdlib"
	"github.com/containous/dyngo/stdlib/syscall"
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
			i.Repl(os.Stdin, os.Stdout)
		}
	} else {
		i := interp.New(opt)
		i.Use(stdlib.Value, stdlib.Type)
		i.Use(syscall.Value, syscall.Type)
		i.Use(interp.ExportValue, interp.ExportType)
		i.Repl(os.Stdin, os.Stdout)
	}
}
