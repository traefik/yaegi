// Package flagplug calls native code that ends the process in Go: flag
// with flag.ExitOnError, and the Fatal method of a native *log.Logger. It
// also uses the global state of package flag (os.Args, flag.CommandLine,
// flag.Usage).
package flagplug

import (
	"context"
	"flag"
	"fmt"
	htmltemplate "html/template"
	"io"
	"log/slog"
	"net/http"
	"os"
	"reflect"
	"strings"
	"text/template"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type parser interface{ Parse([]string) error }

// embedded promotes the methods of a *flag.FlagSet.
type embedded struct{ *flag.FlagSet }

// executer is the Execute method of text/template and html/template.
type executer interface {
	Execute(io.Writer, any) error
}

// execute runs t on fs and a bad flag: the native template calls Parse
// through reflect.
func execute(w io.Writer, t executer, fs *flag.FlagSet) {
	data := struct {
		FS  *flag.FlagSet
		Bad []string
	}{fs, []string{"-bogus"}}
	if err := t.Execute(w, data); err != nil {
		fmt.Fprintln(w, "template:", err)
	}
}

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fs := flag.NewFlagSet("x", flag.ExitOnError)
		fs.SetOutput(w)
		fs.Bool("v", false, "verbose")
		switch r.URL.Path {
		case "/exit":
			os.Exit(2)
		case "/bad":
			fs.Parse([]string{"-bogus"})
		case "/help":
			fs.Parse([]string{"-h"})
		case "/value":
			parse := fs.Parse // a method value
			parse([]string{"-bogus"})
		case "/iface":
			var p parser = fs // a call through an interface
			p.Parse([]string{"-bogus"})
		case "/init":
			var fs2 flag.FlagSet
			fs2.Init("y", flag.ExitOnError)
			fs2.SetOutput(w)
			fs2.Parse([]string{"-bogus"})
		case "/template":
			execute(w, template.Must(template.New("t").Parse("{{.FS.Parse .Bad}}")), fs)
		case "/htmltemplate":
			execute(w, htmltemplate.Must(htmltemplate.New("t").Parse("{{.FS.Parse .Bad}}")), fs)
		case "/reflect":
			out := reflect.ValueOf(fs).MethodByName("Parse").Call([]reflect.Value{reflect.ValueOf([]string{"-bogus"})})
			fmt.Fprintln(w, "reflect:", out[0])
		case "/reflectinit":
			// Init through reflect, then a static Parse.
			var fs2 flag.FlagSet
			reflect.ValueOf(&fs2).MethodByName("Init").Call([]reflect.Value{reflect.ValueOf("y"), reflect.ValueOf(flag.ExitOnError)})
			fs2.SetOutput(w)
			fs2.Parse([]string{"-bogus"})
		case "/reflectboth":
			// Init and Parse through reflect.
			var fs2 flag.FlagSet
			v := reflect.ValueOf(&fs2)
			v.MethodByName("Init").Call([]reflect.Value{reflect.ValueOf("y"), reflect.ValueOf(flag.ExitOnError)})
			fs2.SetOutput(w)
			res := v.MethodByName("Parse").Call([]reflect.Value{reflect.ValueOf([]string{"-bogus"})})
			fmt.Fprintln(w, "reflect:", res[0])
		case "/reflecttype":
			// Init and Parse through reflect.Type.MethodByName.
			var fs2 flag.FlagSet
			t := reflect.TypeOf(&fs2)
			m, _ := t.MethodByName("Init")
			m.Func.Call([]reflect.Value{reflect.ValueOf(&fs2), reflect.ValueOf("y"), reflect.ValueOf(flag.ExitOnError)})
			fs2.SetOutput(w)
			m, _ = t.MethodByName("Parse")
			m.Func.Call([]reflect.Value{reflect.ValueOf(&fs2), reflect.ValueOf([]string{"-bogus"})})
		case "/reflectindex":
			// Init and Parse through reflect.Value.Method(i).
			var fs2 flag.FlagSet
			v := reflect.ValueOf(&fs2)
			m, _ := v.Type().MethodByName("Init")
			v.Method(m.Index).Call([]reflect.Value{reflect.ValueOf("y"), reflect.ValueOf(flag.ExitOnError)})
			fs2.SetOutput(w)
			m, _ = v.Type().MethodByName("Parse")
			v.Method(m.Index).Call([]reflect.Value{reflect.ValueOf([]string{"-bogus"})})
		case "/reflectembed":
			// Init and Parse promoted from an embedded *flag.FlagSet,
			// through reflect.
			e := embedded{new(flag.FlagSet)}
			v := reflect.ValueOf(e)
			v.MethodByName("Init").Call([]reflect.Value{reflect.ValueOf("y"), reflect.ValueOf(flag.ExitOnError)})
			e.SetOutput(w)
			v.MethodByName("Parse").Call([]reflect.Value{reflect.ValueOf([]string{"-bogus"})})
		case "/reflectlogger":
			l := slog.NewLogLogger(slog.Default().Handler(), slog.LevelInfo)
			reflect.ValueOf(l).MethodByName("Fatal").Call([]reflect.Value{reflect.ValueOf("logger fatal")})
		case "/errorhandling":
			// The plugin sees the mode it asked for.
			var fs2 flag.FlagSet
			fs2.Init("y", flag.ExitOnError)
			fmt.Fprintln(w, fs.ErrorHandling() == flag.ExitOnError, fs2.ErrorHandling() == flag.ExitOnError,
				flag.CommandLine.ErrorHandling() == flag.ExitOnError)
		case "/templatelogger":
			// text/template calls methods with one or two results only:
			// Fatal is refused.
			data := struct{ L any }{slog.NewLogLogger(slog.Default().Handler(), slog.LevelInfo)}
			t := template.Must(template.New("t").Parse(`{{.L.Fatal "x"}}`))
			if err := t.Execute(w, data); err != nil {
				fmt.Fprintln(w, "template:", err)
			}
		case "/ok":
			err := fs.Parse([]string{"-v", "a"})
			fmt.Fprintln(w, err, fs.Lookup("v").Value, fs.Args(), fs.ErrorHandling() == flag.ExitOnError)
		case "/logger":
			slog.NewLogLogger(slog.Default().Handler(), slog.LevelInfo).Fatal("logger fatal")
		case "/cmdline":
			// The plugin's flag state: the host's flag.CommandLine,
			// flag.Usage and os.Args do not change.
			if flag.Lookup("plugonly") == nil {
				// In AOT mode, the plugins share the CommandLine of the
				// AOT copy of flag: a second load finds the flag defined.
				flag.Bool("plugonly", false, "a flag of the plugin")
			}
			flag.Usage = func() { fmt.Fprintln(w, "plugin usage") }
			flag.CommandLine.SetOutput(w)
			os.Args = []string{"plug", "-plugonly", "rest"}
			flag.Parse()
			fmt.Fprintln(w, flag.Lookup("plugonly").Value, flag.Parsed(), flag.Args(), flag.CommandLine.Name())
			os.Args = []string{"plug", "-nope"}
			flag.Parse() // flag.Usage, then the restricted os.Exit(2)
		case "/args":
			fmt.Fprint(w, strings.Join(os.Args, " "))
		}
	}), nil
}
