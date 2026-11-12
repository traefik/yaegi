// text/template with program types: fields, methods on value and pointer
// receivers, methods with arguments and errors, maps, range, with,
// if/else, pipelines, variables, custom funcs, nested templates, and the
// errors of parsing and of execution.
package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"text/template"
)

type User struct {
	Name   string
	Age    int
	Tags   []string
	Meta   map[string]int
	Friend *User
	Admin  bool
}

func (u User) Greeting() string        { return "Hello, " + u.Name }
func (u User) Older(n int) int         { return u.Age + n }
func (u *User) Rename(s string) string { u.Name = s; return "renamed" }
func (u User) Check(ok bool) (string, error) {
	if !ok {
		return "", errors.New("check failed for " + u.Name)
	}
	return "checked", nil
}

type Money int64

func (m Money) String() string { return fmt.Sprintf("$%d.%02d", m/100, m%100) }

type List []User

func (l List) Len() int { return len(l) }

func run(name, text string, data any, funcs template.FuncMap) {
	t := template.New(name)
	if funcs != nil {
		t = t.Funcs(funcs)
	}
	t, err := t.Parse(text)
	if err != nil {
		fmt.Println("parse error:", err)
		return
	}
	var sb strings.Builder
	err = t.Execute(&sb, data)
	fmt.Printf("%s: %q err=%v\n", name, sb.String(), err)
}

func main() {
	bob := &User{Name: "Bob", Age: 40}
	ann := User{Name: "Ann", Age: 30, Tags: []string{"x", "y", "z"}, Meta: map[string]int{"b": 2, "a": 1}, Friend: bob, Admin: true}
	funcs := template.FuncMap{
		"upper": strings.ToUpper,
		"add":   func(a, b int) int { return a + b },
		"join":  strings.Join,
		"money": func(i int) Money { return Money(i) },
		"fail":  func() (string, error) { return "", errors.New("custom func failed") },
		"user":  func(n string) User { return User{Name: n} },
	}
	run("fields", "{{.Name}} {{.Age}} {{.Friend.Name}} {{.Friend.Age}} {{.Admin}}", ann, nil)
	run("methods", "{{.Greeting}} {{.Older 5}} {{.Friend.Greeting}} {{.Check true}}", ann, nil)
	run("ptrmethod", "{{.Rename \"Zed\"}} {{.Name}}", bob, nil)
	run("ptrmethod-value", "{{.Rename \"Zed\"}}", ann, nil)
	run("methoderr", "a{{.Check false}}b", ann, nil)
	run("range", "{{range $i, $t := .Tags}}[{{$i}}:{{$t}}]{{else}}none{{end}}", ann, nil)
	run("range-empty", "{{range .Tags}}x{{else}}none{{end}}", User{}, nil)
	run("range-map", "{{range $k, $v := .Meta}}{{$k}}={{$v}};{{end}}", ann, nil)
	run("range-int", "{{range 3}}{{.}},{{end}}", nil, nil)
	run("range-break", "{{range .Tags}}{{if eq . \"y\"}}{{break}}{{end}}{{.}}{{end}}", ann, nil)
	run("with", "{{with .Friend}}{{.Name}} is friend{{else}}no friend{{end}}", ann, nil)
	run("with-nil", "{{with .Friend}}{{.Name}}{{else}}no friend{{end}}", User{}, nil)
	run("if", "{{if and .Admin (gt .Age 18)}}adult admin{{else if .Admin}}young admin{{else}}user{{end}}", ann, nil)
	run("pipeline", "{{.Name | upper | printf \"%s!\"}} {{add .Age 1 | add 100}}", ann, funcs)
	run("vars", "{{$n := .Name}}{{$n = upper $n}}{{$n}} {{len .Tags}} {{index .Tags 1}} {{index .Meta \"b\"}} {{slice .Tags 1 2}}", ann, funcs)
	run("stringer", "{{money 12345}} {{printf \"%v|%s|%d\" (money 5) (money 99) (money 7)}}", nil, funcs)
	run("funcs", "{{join .Tags \"-\"}} {{(user \"Tmp\").Greeting}}", ann, funcs)
	run("funcerr", "x{{fail}}y", nil, funcs)
	run("compare", "{{eq 1 1}} {{ne \"a\" \"b\"}} {{lt 1 2}} {{le 2 2}} {{gt 3.5 1.0}} {{eq .Age 30 40}} {{not .Admin}} {{or 0 \"\" \"x\"}}", ann, nil)
	run("builtins", "{{html \"<a&b>\"}} {{js \"it's\"}} {{urlquery \"a b&c\"}} {{print 1 2 \"s\"}} {{println \"z\"}}", nil, nil)
	run("list", "{{.Len}} {{range .}}{{.Name}},{{end}}", List{{Name: "a"}, {Name: "b"}}, nil)
	run("map-data", "{{.k1}} {{.missing}} {{range $k, $v := .}}{{$k}}{{end}}", map[string]int{"k1": 1, "k2": 2}, nil)
	run("trim", "a  {{- 1 -}}  b {{/* comment */}} c", nil, nil)
	run("define", `{{define "item"}}<{{.}}>{{end}}{{range .Tags}}{{template "item" .}}{{end}}{{block "b" .Name}}[{{.}}]{{end}}`, ann, nil)
	// Errors.
	run("nofield", "{{.Nope}}", ann, nil)
	run("nilptr", "{{.Friend.Name}}", User{}, nil)
	run("badindex", "{{index .Tags 10}}", ann, nil)
	run("wrongargs", "{{.Older}}", ann, nil)
	run("badtype", "{{.Older \"x\"}}", ann, nil)
	run("parse1", "{{.Name", ann, nil)
	run("parse2", "{{if}}x{{end}}", ann, nil)
	run("parse3", "{{nofunc 1}}", ann, nil)
	run("parse4", "{{end}}", ann, nil)
	run("missingkey", "{{.a}}", map[string]int{}, nil)
	t := template.Must(template.New("mk").Option("missingkey=error").Parse("{{.a}}"))
	err := t.Execute(os.Stdout, map[string]int{})
	fmt.Println("\nmissingkey=error:", err)
	var ee template.ExecError
	fmt.Println(errors.As(err, &ee), ee.Name)
	// Lookup and DefinedTemplates.
	root := template.Must(template.New("root").Parse(`{{define "a"}}A{{end}}{{define "b"}}B{{template "a"}}{{end}}root`))
	fmt.Println(root.Lookup("b") != nil, root.Lookup("zz") == nil, len(root.Templates()))
	var sb strings.Builder
	fmt.Println(root.ExecuteTemplate(&sb, "b", nil), sb.String())
	fmt.Println(root.ExecuteTemplate(&sb, "zz", nil))
	fmt.Println(template.HTMLEscapeString("<x>"), template.JSEscapeString("a'b\"c"), template.URLQueryEscaper("a b", 1))
	defer func() { fmt.Println("recovered:", recover()) }()
	template.Must(template.New("bad").Parse("{{"))
}
