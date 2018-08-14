package main

//go:generate go generate github.com/containous/dyngo/export

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/containous/dyngo/export"
	"github.com/containous/dyngo/interp"
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
		// To run test/plugin1.go or test/plugin2.go
		p := &Plugin{"sample", "Middleware", 0, nil}
		p.Syms = i.Exports[p.Pkgname]
		ns := (*i.Expval[p.Pkgname])["NewSample"]
		rarg := []reflect.Value{reflect.ValueOf("test")}
		res := ns.Call(rarg)
		p.Id = res[0].Interface().(int)
		log.Println("res:", res, p.Id)
		http.HandleFunc("/", p.Handler)
		http.ListenAndServe(":8080", nil)
	*/

	/*
		// To run test.plugin0.go
		log.Println("frame:", i.Frame)
		p := &Plugin{"sample", "Middleware", i, nil}
		p.Syms = i.Exports[p.Pkgname]
		log.Println("p.Syms:", p.Syms)
		http.HandleFunc("/", p.Handler)
		http.ListenAndServe(":8080", nil)
	*/
}

// Plugin struct stores metadata for external modules
type Plugin struct {
	Pkgname, Typename string
	ID                int
	Syms              *interp.BinMap
}

// Handler redirect http.Handler processing in the interpreter
func (p *Plugin) Handler(w http.ResponseWriter, r *http.Request) {
	(*p.Syms)["WrapHandler"].(func(int, http.ResponseWriter, *http.Request))(p.ID, w, r)
}
