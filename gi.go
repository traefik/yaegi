package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"strings"
	"time"

	"github.com/containous/gi/interp"
)

func main() {
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

	var b []byte
	var err error
	if len(args) > 0 && args[0] != "-" {
		b, err = ioutil.ReadFile(args[0])
	} else {
		b, err = ioutil.ReadAll(os.Stdin)
	}
	if err != nil {
		panic("Could not read gi source")
	}
	s := string(b)
	if s[:2] == "#!" {
		s = strings.Replace(s, "#!", "//", 1)
	}
	i := interp.NewInterpreter(opt)
	i.AddImport("fmt", "Println", fmt.Println, 0)
	i.AddImport("math", "Pi", math.Pi, 0)
	i.AddImport("math", "Cos", math.Cos, 1)
	i.AddImport("time", "Now", time.Now, 1)
	i.Eval(string(s))
}
