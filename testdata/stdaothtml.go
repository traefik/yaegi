package main

// html/template in default mode runs as its AOT copy, with the copy of
// text/template (stdAOTModes in vm/aot.go): the native package failed on
// the methods of the program's types.
// (From the reflect battery of the modes challenge.)
//
// html/template: fields and methods of interpreted types, FuncMap with
// interpreted funcs, contextual escaping, template.HTML from a method.

import (
	"fmt"
	"html/template"
	"os"
	"strings"
)

type Page struct {
	Title string
	Items []Item
	URL   string
	JS    string
}

type Item struct {
	Name  string
	Price int
}

func (i Item) Label() string           { return fmt.Sprintf("%s <%d>", i.Name, i.Price) }
func (i Item) Bold() template.HTML     { return template.HTML("<b>" + i.Name + "</b>") }
func (p *Page) Count() int             { return len(p.Items) }
func (p Page) Safe() template.URL      { return template.URL(p.URL) }
func (p Page) Attr() template.HTMLAttr { return template.HTMLAttr(`data-x="1"`) }

type Money int

func (m Money) String() string { return fmt.Sprintf("%d EUR & co", int(m)) }

func main() {
	funcs := template.FuncMap{
		"lower": strings.ToLower,
		"twice": func(s string) string { return s + s },
		"money": func(i int) Money { return Money(i) },
	}
	const src = `<h1 title="{{.Title}}">{{.Title}}</h1>
<ul>{{range .Items}}<li>{{.Label}} {{.Bold}} {{money .Price}}</li>{{end}}</ul>
<a href="/search?q={{.Title}}">x</a> <a href="{{.URL}}">u</a> <a href="{{.Safe}}">s</a>
<p {{.Attr}}>{{.Count}} {{twice (lower .Title)}}</p>
<script>var t = {{.Title}}; var js = {{.JS}};</script>
`
	t := template.Must(template.New("p").Funcs(funcs).Parse(src))
	p := &Page{Title: `Tom & "Jerry" <script>`, Items: []Item{{"a<b", 1}, {"c", 2}}, URL: "javascript:alert(1)", JS: "</script>"}
	err := t.Execute(os.Stdout, p)
	fmt.Println("err:", err)
	err = t.Execute(os.Stdout, *p)
	fmt.Println("err:", err != nil)
}
