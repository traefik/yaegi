package main

import (
	"flag"
	"io/ioutil"
	"os"
	"strings"

	"github.com/containous/gi/interp"
)

func main() {
	opt := interp.InterpOpt{}
	flag.BoolVar(&opt.Ast, "a", false, "display AST graph")
	flag.BoolVar(&opt.Cfg, "c", false, "display CFG graph")
	flag.Parse()
	args := flag.Args()

	var b []byte
	var err error
	if len(args) > 0 {
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
	i.Eval(string(s))
}
