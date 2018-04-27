package main

//go:generate go generate github.com/containous/gi/export

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"os"
	"strings"
	"time"

	"github.com/containous/gi/export"
	"github.com/containous/gi/interp"
)

func main() {
	p := export.Pkg
	log.Println((*p)["fmt"])
	opt := interp.InterpOpt{}
	flag.BoolVar(&opt.Ast, "a", false, "display AST graph")
	flag.BoolVar(&opt.Cfg, "c", false, "display CFG graph")
	flag.BoolVar(&opt.NoRun, "n", false, "do not run")
	flag.Usage = func() {
		fmt.Println("Usage:", os.Args[0], "[options] [script|-]] [args]")
		fmt.Println("Options:")
		flag.PrintDefaults()
	}
	flag.Parse()
	args := flag.Args()
	log.SetFlags(log.Lshortfile)

	var b []byte
	var err error
	if len(args) > 0 && args[0] != "-" {
		b, err = ioutil.ReadFile(args[0])
	} else {
		b, err = ioutil.ReadAll(os.Stdin)
	}
	if err != nil {
		log.Fatal("Could not read file: ", args[0])
	}
	s := string(b)
	if s[:2] == "#!" {
		s = strings.Replace(s, "#!", "//", 1)
	}
	i := interp.NewInterpreter(opt)
	i.AddImport("fmt", "Println", fmt.Println)
	i.AddImport("math", "Pi", math.Pi)
	i.AddImport("math", "Cos", math.Cos)
	i.AddImport("time", "Now", time.Now)
	i.AddImport("time", "Time", new(time.Time))
	i.AddImport("time", "Month", new(time.Month))
	i.Eval(string(s))
}
