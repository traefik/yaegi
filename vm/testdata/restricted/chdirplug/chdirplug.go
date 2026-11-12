// Package chdirplug tries to change the working directory of the host with
// (*os.File).Chdir without naming it in its code: the load-time check sees
// only the functions the code names (restrict.go), so each path below must
// be refused when it runs.
package chdirplug

import (
	"context"
	htmltemplate "html/template"
	"io"
	"net/http"
	"os"
	"reflect"
	"text/template"
	"text/template/parse"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type chdirer interface{ Chdir() error }

// embeds promotes the methods of *os.File.
type embeds struct{ *os.File }

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f, err := os.Open(r.URL.Query().Get("dir"))
		if err != nil {
			panic(err)
		}
		defer f.Close()
		var any_ any = f
		switch r.URL.Path {
		case "/methodbyname":
			reflect.ValueOf(f).MethodByName("Chdir").Call(nil)
		case "/method":
			v := reflect.ValueOf(f)
			for i := 0; i < v.NumMethod(); i++ {
				if v.Type().Method(i).Name == "Chdir" {
					v.Method(i).Call(nil)
				}
			}
		case "/methodinterface":
			reflect.ValueOf(f).MethodByName("Chdir").Interface().(func() error)()
		case "/typemethod":
			m, _ := reflect.TypeOf(f).MethodByName("Chdir")
			m.Func.Call([]reflect.Value{reflect.ValueOf(f)})
		case "/typemethodindex":
			t := reflect.TypeOf(f)
			for i := 0; i < t.NumMethod(); i++ {
				if t.Method(i).Name == "Chdir" {
					t.Method(i).Func.Call([]reflect.Value{reflect.ValueOf(f)})
				}
			}
		case "/interface":
			any_.(chdirer).Chdir()
		case "/embedreflect":
			reflect.ValueOf(embeds{f}).MethodByName("Chdir").Call(nil)
		case "/template":
			t := template.Must(template.New("t").Parse("{{.Chdir}}"))
			if err := t.Execute(io.Discard, f); err != nil {
				panic(err)
			}
		case "/templatemap":
			t := template.Must(template.New("t").Parse(`{{define "x"}}{{.F.Chdir}}{{end}}{{template "x" .}}`))
			if err := t.ExecuteTemplate(io.Discard, "t", map[string]any{"F": f}); err != nil {
				panic(err)
			}
		case "/htmltemplate":
			t := htmltemplate.Must(htmltemplate.New("t").Parse("{{with $x := .}}{{$x.Chdir}}{{end}}"))
			if err := t.Execute(io.Discard, f); err != nil {
				panic(err)
			}
		case "/templatetree":
			// A FuncMap function changes the tree while the template runs.
			var t *template.Template
			t = template.Must(template.New("t").Funcs(template.FuncMap{"add": func() string {
				t.Tree.Root.Nodes[1].(*parse.ActionNode).Pipe.Cmds[0].Args[0].(*parse.FieldNode).Ident[0] = "Chdir"
				return ""
			}}).Parse("{{add}}{{.Name}}"))
			if err := t.Execute(io.Discard, f); err != nil {
				panic(err)
			}
		case "/htmltemplatetree":
			var t *htmltemplate.Template
			t = htmltemplate.Must(htmltemplate.New("t").Funcs(htmltemplate.FuncMap{"add": func() string {
				t.Tree.Root.Nodes[1].(*parse.ActionNode).Pipe.Cmds[0].Args[0].(*parse.FieldNode).Ident[0] = "Chdir"
				return ""
			}}).Parse("{{add}}{{.Name}}"))
			if err := t.Execute(io.Discard, f); err != nil {
				panic(err)
			}
		case "/templateadd":
			// A FuncMap function adds a template while the template runs,
			// and {{template}} looks it up at run time.
			var t *template.Template
			t = template.Must(template.New("t").Funcs(template.FuncMap{"add": func() string {
				template.Must(t.New("x").Parse("{{.Chdir}}"))
				return ""
			}}).Parse(`{{add}}{{template "x" .}}`))
			if err := t.Execute(io.Discard, f); err != nil {
				panic(err)
			}
		case "/templatenested":
			// The template calls Execute of another native template, with
			// the native reflect.
			inner := template.Must(template.New("i").Parse("{{.Chdir}}"))
			t := template.Must(template.New("t").Parse("{{.T.Execute .W .F}}"))
			if err := t.Execute(io.Discard, map[string]any{"T": inner, "W": io.Discard, "F": f}); err != nil {
				panic(err)
			}
		case "/templateok":
			// A template that calls no refused method still runs.
			t := template.Must(template.New("t").Parse("{{.Name}}"))
			if err := t.Execute(w, f); err != nil {
				panic(err)
			}
		}
	}), nil
}
