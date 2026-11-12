// html/template with program types: contextual escaping of fields and
// method results in text, attributes, URLs, scripts and styles, the safe
// types (template.HTML, URL, JS), custom funcs, and escaping errors.
package main

import (
	"fmt"
	"html/template"
	"strings"
)

type Post struct {
	Title   string
	Body    template.HTML
	Link    string
	SafeURL template.URL
	Tags    []string
	Author  *Author
	Score   float64
	Data    map[string]any
}

type Author struct{ Name, Email string }

func (a *Author) Display() string { return a.Name + " <" + a.Email + ">" }
func (p Post) Summary(n int) string {
	if len(p.Title) > n {
		return p.Title[:n] + "..."
	}
	return p.Title
}

func run(name, text string, data any) {
	t, err := template.New(name).Funcs(template.FuncMap{
		"lower": strings.ToLower,
		"raw":   func(s string) template.HTML { return template.HTML(s) },
	}).Parse(text)
	if err != nil {
		fmt.Println("parse error:", err)
		return
	}
	var sb strings.Builder
	err = t.Execute(&sb, data)
	fmt.Printf("%s:\n  %s\n  err=%v\n", name, sb.String(), err)
}

func main() {
	p := Post{
		Title:   `<script>alert("x")</script> & 'quotes'`,
		Body:    template.HTML("<b>bold</b>"),
		Link:    "javascript:alert(1)",
		SafeURL: template.URL("javascript:ok()"),
		Tags:    []string{"a<b", "c&d", `e"f`},
		Author:  &Author{"Ann", "ann@example.com"},
		Score:   1.5,
		Data:    map[string]any{"k": "v</script>", "n": 3, "l": []int{1, 2}},
	}
	run("text", "<p>{{.Title}}</p><div>{{.Body}}</div>", p)
	run("attr", `<a title="{{.Title}}" href="{{.Link}}">x</a><a href="{{.SafeURL}}">y</a>`, p)
	run("url", `<a href="/search?q={{.Title}}&t={{index .Tags 0}}">s</a><a href="http://h/{{.Title}}">p</a>`, p)
	run("script", `<script>var t = {{.Title}}; var d = {{.Data}}; var s = {{.Score}}; var l = {{.Tags}};</script>`, p)
	run("style", `<style>p { color: {{"red"}}; font: {{.Title}} }</style>`, p)
	run("method", `<p>{{.Author.Display}} {{.Summary 5}} {{.Summary 100 | lower}}</p>`, p)
	run("range", `<ul>{{range .Tags}}<li class="{{.}}">{{.}}</li>{{end}}</ul>`, p)
	run("funcs", `{{raw "<i>trusted</i>"}} {{lower "<B>"}}`, p)
	run("unquoted-attr", `<input value={{.Title}}>`, p)
	run("onclick", `<button onclick="f({{.Title}})">b</button>`, p)
	run("comment", `<!-- {{.Title}} -->x`, p)
	run("nil-author", `{{with .Author}}{{.Name}}{{else}}anon{{end}}`, Post{})
	run("err-method", `{{.Author.Display}}`, Post{})
	run("err-branch", `<a {{if .Score}}href="{{.Link}}"{{end}}>`, p)
	run("err-unclosed", `<a href="{{.Link}}`, p)
	run("err-parse", `{{.Title`, p)
	t := template.Must(template.New("x").Parse(`{{define "T"}}<b>{{.}}</b>{{end}}`))
	var sb strings.Builder
	fmt.Println(t.ExecuteTemplate(&sb, "T", "<i>"), sb.String())
	c, err := t.Clone()
	fmt.Println(err, c == nil)
	t2 := template.Must(template.New("y").Parse(`{{define "T"}}<i>{{.}}</i>{{end}}`))
	c, err = t2.Clone()
	fmt.Println(err, c.Name())
	sb.Reset()
	fmt.Println(c.ExecuteTemplate(&sb, "T", "a&b"), sb.String())
	fmt.Println(template.HTMLEscapeString(`<"'&>`), template.JSEscapeString(`</script>`), template.URLQueryEscaper("a/b c"))
}
