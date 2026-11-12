package main

// A program that imports text/template, html/template or log/slog runs the
// standard library as in AOT mode, also in default mode (stdAOTModes in
// vm/aot.go): the native template package saw native copies without the
// methods of the program's types and failed on {{.Greeting}}.
// (From the reflect battery of the modes challenge.)
//
// text/template: fields and methods of interpreted types (value and
// pointer receivers, with arguments, returning errors), FuncMap with
// interpreted funcs, range over interpreted slices and maps.

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"text/template"
)

type User struct {
	Name  string
	Age   int
	Tags  []string
	Boss  *User
	Props map[string]Prop
	priv  int
}

type Prop struct{ V string }

func (p Prop) Upper() string { return strings.ToUpper(p.V) }

func (u User) Greeting() string         { return "Hello, " + u.Name }
func (u User) Older(n int) int          { return u.Age + n }
func (u *User) PtrName() string         { return "*" + u.Name }
func (u User) Check() (string, error)   { return "checked", nil }
func (u User) Fail() (string, error)    { return "", errors.New("fail called") }
func (u User) Self() User               { return u }
func (u User) Fn() func(int) int        { return func(i int) int { return i * 2 } }
func (u User) Format(sep string) string { return strings.Join(u.Tags, sep) }

type Stringy int

func (s Stringy) String() string { return fmt.Sprintf("S%d", int(s)) }

func main() {
	funcs := template.FuncMap{
		"shout": func(s string) string { return strings.ToUpper(s) + "!" },
		"add":   func(a, b int) int { return a + b },
		"user":  func(n string) User { return User{Name: n} },
		"pair":  func(a, b any) []any { return []any{a, b} },
		"err":   func() (string, error) { return "", errors.New("func error") },
	}
	const src = `{{.Name}} {{.Age}} {{.Greeting}} {{.Older 5}} {{.PtrName}} {{.Check}}
{{range $i, $t := .Tags}}[{{$i}}:{{$t}}]{{end}}
{{with .Boss}}boss={{.Name}} {{.Greeting}}{{end}}
{{range $k, $v := .Props}}{{$k}}={{$v.V}}/{{$v.Upper}} {{end}}
{{shout .Name}} {{add 1 2}} {{(user "Zed").Greeting}} {{.Self.Self.Name}}
{{call .Fn 21}} {{.Format "+"}} {{printf "%v|%s|%d" .Stringy .Stringy .Age}}
{{index .Tags 1}} {{len .Props}} {{pair 1 "a"}}
{{if eq .Name "Ann"}}yes{{else}}no{{end}} {{.Name | shout | printf "%q"}}
`
	t := template.Must(template.New("t").Funcs(funcs).Parse(src))
	type data struct {
		User
		Stringy Stringy
	}
	u := &User{Name: "Ann", Age: 30, Tags: []string{"a", "b"}, Boss: &User{Name: "Bo"},
		Props: map[string]Prop{"x": {"lo"}, "y": {"hi"}}, priv: 1}
	err := t.Execute(os.Stdout, data{User: *u, Stringy: 7})
	fmt.Println("err:", err)
	// Pointer data: PtrName in the method set.
	err = template.Must(template.New("p").Parse("{{.PtrName}} {{.Greeting}}\n")).Execute(os.Stdout, u)
	fmt.Println("err:", err)
	// Errors.
	err = template.Must(template.New("f").Parse("{{.Fail}}\n")).Execute(os.Stdout, u)
	fmt.Println("err:", err)
	err = template.Must(template.New("g").Funcs(funcs).Parse("{{err}}\n")).Execute(os.Stdout, u)
	fmt.Println("err:", err)
	err = template.Must(template.New("h").Parse("{{.priv}}\n")).Execute(os.Stdout, u)
	fmt.Println("err:", err)
	err = template.Must(template.New("i").Parse("{{.Missing}}\n")).Execute(os.Stdout, u)
	fmt.Println("err:", err)
}
