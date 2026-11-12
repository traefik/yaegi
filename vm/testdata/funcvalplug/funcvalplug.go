// Package funcvalplug reaches the functions and methods that a restricted
// plugin gets in its own version (the environment, os.Exit, log, log/slog,
// flag, http.DefaultServeMux) through values: a func value, a func
// stored in a map, a func given to native code that calls it, a method
// value, a method expression, and reflect. Each must be the plugin's
// version, as a direct call is: the host keeps its state.
package funcvalplug

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"
	"sync"
)

func try(out *strings.Builder, name string, f func()) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(out, "%s: panic %v\n", name, r)
		}
	}()
	f()
	fmt.Fprintf(out, "%s: ok\n", name)
}

func Run() string {
	var out strings.Builder

	// the environment
	set := os.Setenv
	set("FUNCVAL_HOST", "value")
	funcs := map[string]func(string, string) error{"set": os.Setenv}
	funcs["set"]("FUNCVAL_MAP", "map")
	reflect.ValueOf(os.Setenv).Call([]reflect.Value{reflect.ValueOf("FUNCVAL_REFLECT"), reflect.ValueOf("reflect")})
	fmt.Fprintln(&out, "env:", os.Getenv("FUNCVAL_HOST"), os.Getenv("FUNCVAL_MAP"), os.Getenv("FUNCVAL_REFLECT"))
	sync.OnceFunc(os.Clearenv)() // called by native code
	fmt.Fprintln(&out, "cleared:", len(os.Environ()))

	// os.Exit and the Fatal functions and methods panic
	try(&out, "exit value", func() { exit := os.Exit; exit(3) })
	try(&out, "exit map", func() { map[int]func(int){0: os.Exit}[0](4) })
	try(&out, "exit reflect", func() { reflect.ValueOf(os.Exit).Call([]reflect.Value{reflect.ValueOf(5)}) })
	try(&out, "exit native", func() { sync.OnceFunc(func() { os.Exit(6) })() })
	log.SetOutput(io.Discard)
	try(&out, "fatal value", func() { f := log.Fatal; f("a") })
	try(&out, "fatal method value", func() { f := log.Default().Fatalf; f("b") })
	try(&out, "fatal method expression", func() { f := (*log.Logger).Fatalf; f(log.Default(), "c") })
	try(&out, "fatal reflect", func() {
		reflect.ValueOf(log.Default()).MethodByName("Fatal").Call([]reflect.Value{reflect.ValueOf("d")})
	})

	// the default loggers of log and slog
	var lb bytes.Buffer
	setOutput := log.SetOutput
	setOutput(&lb)
	setFlags := log.Default().SetFlags
	setFlags(0)
	log.Print("log")
	var sb bytes.Buffer
	setDefault := slog.SetDefault
	setDefault(slog.New(slog.NewTextHandler(&sb, nil)))
	setLevel := slog.SetLogLoggerLevel
	setLevel(slog.LevelError)
	info := slog.Info
	info("slog")
	fmt.Fprintf(&out, "log: %q %v\n", lb.String(), sb.Len() > 0)

	// flag: CommandLine and ExitOnError
	define := flag.String
	define("funcval-flag", "", "")
	parse := flag.CommandLine.Parse
	flag.CommandLine.SetOutput(io.Discard)
	try(&out, "parse method value", func() { parse([]string{"-nope"}) })
	parseExpr := (*flag.FlagSet).Parse
	try(&out, "parse method expression", func() {
		fs := flag.NewFlagSet("x", flag.ExitOnError)
		fs.SetOutput(io.Discard)
		parseExpr(fs, []string{"-nope"})
	})
	fmt.Fprintln(&out, "flag:", flag.Lookup("funcval-flag") != nil)

	// http.DefaultServeMux
	handle := http.HandleFunc
	handle("/funcval", func(http.ResponseWriter, *http.Request) {})
	_, pattern := http.DefaultServeMux.Handler(&http.Request{Method: "GET", URL: mustURL("/funcval")})
	fmt.Fprintln(&out, "mux:", pattern)

	// context.AfterFunc calls the plugin's function on a goroutine of its
	// own
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	context.AfterFunc(ctx, func() { set("FUNCVAL_AFTER", "after"); close(done) })
	cancel()
	<-done
	fmt.Fprintln(&out, "after:", os.Getenv("FUNCVAL_AFTER"))
	return out.String()
}

func mustURL(path string) *url.URL {
	u, _ := url.Parse(path)
	return u
}
