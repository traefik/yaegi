package main

import (
	"log"
	"net/http"

	"github.com/containous/dyngo/interp"
	"github.com/containous/dyngo/stdlib"
)

// This program starts an interpreter which loads a plugin to handle HTTP requests

func main() {
	log.SetFlags(log.Lshortfile) // Debug: print source file locations in log output

	// Init go interpreter
	i := interp.New(interp.Opt{})
	i.Use(stdlib.Value, stdlib.Type) // Use binary standard library

	// Load plugin from sources
	_, err := i.Eval(`import "github.com/containous/dyngo/example/test_plugin/plugin"`)
	if err != nil {
		log.Fatal(err)
	}

	// Obtain a HTTP handler from the plugin
	value, err := i.Eval(`plugin.NewSample("test")`)
	if err != nil {
		log.Fatal(err)
	}
	handler := value.Interface().(func(http.ResponseWriter, *http.Request))

	http.HandleFunc("/", handler)
	err = http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal(err)
	}
}
