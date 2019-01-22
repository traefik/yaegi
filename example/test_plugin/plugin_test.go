package main

import (
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/containous/dyngo/interp"
	"github.com/containous/dyngo/stdlib"
)

func TestPlugin(t *testing.T) {
	log.SetFlags(log.Lshortfile) // Debug: print source file locations in log output

	// Init go interpreter
	i := interp.New(interp.Opt{})
	i.Use(stdlib.Value, stdlib.Type) // Use binary standard library

	// Load plugin from sources
	if _, err := i.Eval(`import "github.com/containous/dyngo/example/test_plugin/plugin"`); err != nil {
		t.Fatal(err)
	}

	// Obtain a HTTP handler from the plugin
	value, err := i.Eval(`plugin.NewSample("test")`)
	if err != nil {
		t.Fatal(err)
	}

	handler := value.Interface().(func(http.ResponseWriter, *http.Request))

	mux := http.NewServeMux()
	mux.HandleFunc("/", handler)

	server := httptest.NewServer(mux)
	defer server.Close()

	resp, err := http.DefaultClient.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	bytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	expected := "Welcome to my website test v1\n"
	if string(bytes) != expected {
		t.Errorf("Got %s, want %s", string(bytes), expected)
	}
}

func TestPluginMethod(t *testing.T) {
	log.SetFlags(log.Lshortfile) // Debug: print source file locations in log output

	// Init go interpreter
	i := interp.New(interp.Opt{})
	i.Use(stdlib.Value, stdlib.Type) // Use binary standard library

	// Load plugin from sources
	if _, err := i.Eval(`import "github.com/containous/dyngo/example/test_plugin/plugin"`); err != nil {
		t.Fatal(err)
	}

	// Obtain a HTTP handler from the plugin
	value, err := i.Eval(`plugin.NewSampleHandler("test")`)
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := value.Interface().(http.Handler); ok {
		t.Fatal("methods can be used, it's not possible")
	}
}
