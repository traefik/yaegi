package main

import (
	"log"
	"net/http"

	"github.com/containous/dyngo/interp"
	"github.com/containous/dyngo/stdlib"
)

// Plugin struct stores metadata for external modules
type Plugin struct {
	name    string
	handler func(http.ResponseWriter, *http.Request)
}

// Handler redirect http.Handler processing in the interpreter
func (p *Plugin) Handler(w http.ResponseWriter, r *http.Request) {
	p.handler(w, r)
}

func main() {
	// Init go interpreter
	log.SetFlags(log.Lshortfile)
	//i := interp.NewInterpreter(interp.Opt{AstDot: true}, "")
	i := interp.NewInterpreter(interp.Opt{}, "")
	i.Import(stdlib.Value, stdlib.Type)

	// Load plugin
	_, err := i.Eval(`import "github.com/containous/dyngo/example/test_plugin/plugin"`)
	log.Println("err:", err)

	handler, err := i.Eval(`plugin.NewSample("test")`)
	log.Println("handler:", handler, "err:", err)
	p := &Plugin{"sample", nil}
	p.handler = handler.Interface().(func(http.ResponseWriter, *http.Request))
	http.HandleFunc("/", p.Handler)
	http.ListenAndServe(":8080", nil)
}
