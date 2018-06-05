package main

//go:generate go generate github.com/containous/gi/export

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/containous/gi/export"
	"github.com/containous/gi/interp"
)

func main() {
	opt := interp.Opt{Entry: "main"}
	flag.BoolVar(&opt.AstDot, "a", false, "display AST graph")
	flag.BoolVar(&opt.CfgDot, "c", false, "display CFG graph")
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
	i.ImportBin(export.Pkg)
	i.Eval(string(s))
	//samp := *i.Exports["sample"]
	//log.Println("exports:", samp)

	/*
		// To run test/export1.go
		p := &Plugin{"sample", "Middleware", i, nil}
		p.Syms = p.Interp.Exports[p.Pkgname]
		http.HandleFunc("/", p.Handler)
		http.ListenAndServe(":8080", nil)
	*/
}

type Plugin struct {
	Pkgname, Typename string
	Interp            *interp.Interpreter
	Syms              *interp.SymMap
}

func (p *Plugin) Handler(w http.ResponseWriter, r *http.Request) {
	(*p.Syms)["Handler"].(func(http.ResponseWriter, *http.Request))(w, r)
}
