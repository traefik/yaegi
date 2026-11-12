// AOT test program: text/template on interpreted types.
//
//aot:lib text/template
package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/template"
	"time"
)

type Level int

func (l Level) String() string { return [...]string{"low", "mid", "high"}[l] }

type User struct {
	Name   string
	Age    int
	Level  Level
	Tags   []string
	Meta   map[string]any
	Friend *User
	admin  bool
}

func (u User) Greeting(prefix string) string { return prefix + ", " + u.Name }
func (u *User) IsAdult() bool                 { return u.Age >= 18 }
func (u User) Fail() (string, error)          { return "", errors.New("method failed") }
func (u User) Pair() (int, int)               { return 1, 2 }

type Money float64

func (m Money) String() string { return fmt.Sprintf("$%.2f", float64(m)) }

const tpl = `{{define "item"}}[{{.}}]{{end -}}
Hello {{.Name}} ({{.Age}}), level={{.Level}} {{printf "%d" .Level}} {{.Level | printf "%v"}}
{{if .IsAdult}}adult{{else}}minor{{end}} {{.Greeting "hi"}} {{with .Friend}}friend={{.Name}}{{end}}
{{range $i, $t := .Tags}}{{if $i}}, {{end}}{{template "item" $t}}{{end}}
{{range $k, $v := .Meta}}{{$k}}={{$v}};{{end}}
{{len .Tags}} {{index .Tags 1}} {{index .Meta "money"}} {{upper .Name}} {{join .Tags "+"}} {{add 2 3}}
{{- /* comment */ -}}
{{$x := .Age}}{{if gt $x 30}} old{{else if eq $x 30}} thirty{{else}} young{{end}}
{{and .Name .Age}} {{or "" "dflt"}} {{not .Age}} {{.Meta.money}} {{call .Meta.fn 4}}
{{range .Friend.Tags}}{{.}}{{else}}no tags{{end}} {{html "<b>"}} {{js "a'b"}} {{urlquery "a b"}}
{{slice .Name 1 3}} {{eq .Level 2}} {{lt 1.5 2.5}}
`

func main() {
	if benchMain(benchOp) {
		return
	}
	funcs := template.FuncMap{
		"upper": strings.ToUpper,
		"join":  strings.Join,
		"add":   func(a, b int) int { return a + b },
	}
	t := template.Must(template.New("t").Funcs(funcs).Parse(tpl))
	u := &User{Name: "Alice", Age: 30, Level: 2, Tags: []string{"x", "y"},
		Meta:   map[string]any{"money": Money(3.5), "n": 1, "fn": func(i int) Level { return Level(i % 3) }},
		Friend: &User{Name: "Bob", Age: 12}, admin: true}
	if err := t.Execute(os.Stdout, u); err != nil {
		fmt.Println("exec error:", err)
	}
	for _, src := range []string{
		`{{.Fail}}`,
		`{{.Missing}}`,
		`{{.admin}}`,
		`{{.Pair}}`,
		`{{index .Tags 5}}`,
		`{{template "nope"}}`,
		`{{.Name.Bad}}`,
	} {
		t2, err := template.New("e").Option("missingkey=error").Parse(src)
		if err != nil {
			fmt.Println("parse:", err)
			continue
		}
		var sb strings.Builder
		err = t2.Execute(&sb, u)
		fmt.Printf("%q %v\n", sb.String(), err)
	}
	m := map[string]Level{"b": 1, "a": 0}
	template.Must(template.New("m").Parse("{{range $k, $v := .}}{{$k}}:{{$v}} {{end}}{{.a}}\n")).Execute(os.Stdout, m)
	ch := make(chan Money, 2)
	ch <- 1
	ch <- 2
	close(ch)
	template.Must(template.New("c").Parse("{{range .}}{{.}} {{end}}\n")).Execute(os.Stdout, ch)
	template.Must(template.New("i").Parse("{{range 3}}{{.}}{{end}} {{.}}\n")).Execute(os.Stdout, []any{Level(1), nil, "s"})
}

var benchTpl = template.Must(template.New("b").Parse(`{{.Name}} {{.Level}} {{range .Tags}}[{{.}}]{{end}} {{if .IsAdult}}adult{{end}} {{.Greeting "hi"}}`))

// benchOp: execute a small template with fields, methods and range.
func benchOp() {
	var sb strings.Builder
	u := &User{Name: "Alice", Age: 30, Level: 2, Tags: []string{"x", "y", "z"}}
	if err := benchTpl.Execute(&sb, u); err != nil {
		panic(err)
	}
}

func benchMain(op func()) bool {
	n, _ := strconv.Atoi(os.Getenv("AOT_BENCH"))
	if n <= 0 {
		return false
	}
	op()
	t0 := time.Now()
	for i := 0; i < n; i++ {
		op()
	}
	fmt.Printf("BENCH %d\n", time.Since(t0).Nanoseconds()/int64(n))
	return true
}
