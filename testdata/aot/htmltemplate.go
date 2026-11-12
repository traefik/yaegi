// AOT test program: html/template on interpreted types.
//
//aot:lib html/template
package main

import (
	"fmt"
	"html/template"
	"os"
	"strconv"
	"strings"
	"time"
)

type Link struct {
	Title string
	URL   string
	Attrs map[string]string
}

type Badge string

func (b Badge) String() string { return "<" + string(b) + ">" }

type Page struct {
	Title  string
	Links  []Link
	Badge  Badge
	Safe   template.HTML
	Script string
	Count  int
	Data   map[string]any
}

func (p Page) Upper(s string) string { return strings.ToUpper(s) }

const page = `<html><head><title>{{.Title}}</title></head>
<body data-count="{{.Count}}">
<h1>{{.Upper .Title}}</h1>
{{range .Links}}<a href="{{.URL}}" title="{{.Title}}"{{range $k, $v := .Attrs}} data-{{$k}}="{{$v}}"{{end}}>{{.Title}}</a>
{{end}}<p>{{.Badge}} {{.Safe}}</p>
<script>var s = {{.Script}}; var d = {{.Data}};</script>
<style>p { color: {{"red"}}; }</style>
<a href="/search?q={{.Title}}&n={{.Count}}">s</a>
</body></html>
`

func main() {
	if benchMain(benchOp) {
		return
	}
	t := template.Must(template.New("p").Parse(page))
	p := Page{
		Title: `Tom & "Jerry" <script>`,
		Links: []Link{{"home", "/home?a=1&b=2", map[string]string{"x": "1"}}, {"evil", "javascript:alert(1)", nil}},
		Badge: "new", Safe: template.HTML("<em>ok</em>"), Script: `</script><b>"x"`, Count: 3,
		Data: map[string]any{"k": []int{1, 2}, "s": Badge("b")},
	}
	if err := t.Execute(os.Stdout, p); err != nil {
		fmt.Println("error:", err)
	}
	fmt.Println(template.HTMLEscapeString(`<a href="x">`), template.JSEscapeString(`'"<`), template.URLQueryEscaper("a b", 1))
	_, err := template.New("bad").Parse(`<a href="{{.}}`)
	fmt.Println(err)
	var sb strings.Builder
	err = template.Must(template.New("x").Parse(`<a {{.}}>`)).Execute(&sb, Badge("x"))
	fmt.Println(sb.String(), err)
}

var benchT = template.Must(template.New("b").Parse(`<ul>{{range .Links}}<li><a href="{{.URL}}">{{.Title}}</a></li>{{end}}</ul><p>{{.Badge}}</p>`))

// benchOp: execute an escaped template.
func benchOp() {
	var sb strings.Builder
	p := Page{Links: []Link{{"a<b", "/x?y=1", nil}, {"c", "/z", nil}}, Badge: "n"}
	if err := benchT.Execute(&sb, p); err != nil {
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
