package main

//go:generate go generate github.com/containous/dyngo/stdlib

import (
	"bufio"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/containous/dyngo/interp"
	"github.com/containous/dyngo/stdlib"
)

func main() {
	opt := interp.Opt{Entry: "main"}
	flag.BoolVar(&opt.AstDot, "a", false, "display AST graph")
	flag.BoolVar(&opt.CfgDot, "c", false, "display CFG graph")
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
		i := interp.NewInterpreter(opt, args[0])
		i.Import(stdlib.Value, stdlib.Type)
		i.Eval(string(s))
	} else {
		i := interp.NewInterpreter(opt, "")
		i.Import(stdlib.Value, stdlib.Type)
		s := bufio.NewScanner(os.Stdin)
		prompt := getPrompt()
		prompt()
		for s.Scan() {
			if v, err := i.Eval(s.Text()); err != nil {
				fmt.Println(err)
			} else if v.IsValid() {
				fmt.Println(v)
			}
			prompt()
		}
	}
	/*
		// To run test/plugin1.go or test/plugin2.go
		p := &Plugin{"sample", "Middleware", 0, nil}
		//p.Syms = i.Exports[p.Pkgname]
		//ns := (*i.Expval[p.Pkgname])["NewSample"]
		ns := i.Export("sample", "NewSample")
		log.Println("ns:", ns)
		rarg := []reflect.Value{reflect.ValueOf("test")}
		res := ns.Call(rarg)
		p.Id = int(res[0].Int())
		p.handler = i.Export("sample", "WrapHandler").Interface().(func(int, http.ResponseWriter, *http.Request))
		log.Println("res:", res, p.Id)
		http.HandleFunc("/", p.Handler)
		http.ListenAndServe(":8080", nil)
	*/
	/*
		// To run test.plugin0.go
		log.Println("frame:", i.Frame)
		p := &Plugin{"sample", "Middleware", 0, nil}
		p.Syms = i.Exports[p.Pkgname]
		log.Println("p.Syms:", p.Syms)
		http.HandleFunc("/", p.Handler)
		http.ListenAndServe(":8080", nil)
	*/
}

/*
// Plugin struct stores metadata for external modules
type Plugin struct {
	Pkgname, Typename string
	Id                int
	handler           func(int, http.ResponseWriter, *http.Request)
}

// Handler redirect http.Handler processing in the interpreter
func (p *Plugin) Handler(w http.ResponseWriter, r *http.Request) {
	p.handler(p.Id, w, r)
}
*/

// getPrompt returns a function which prints a prompt only if stdin is a terminal
func getPrompt() func() {
	if stat, err := os.Stdin.Stat(); err == nil && stat.Mode()&os.ModeCharDevice != 0 {
		return func() { fmt.Print("> ") }
	}
	return func() {}
}
