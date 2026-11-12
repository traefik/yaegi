// Native boundary of html/template: program types given to html/template
// (docs/BOUNDARY.md).
package main

import (
	"fmt"
	"html/template"
	"strings"
)

var cur string

// probe runs one probe: its lines start with its id, and its panic is printed.
func probe(id string, f func()) {
	cur = id
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("%s: panic: %T: %v\n", id, r, r)
		}
	}()
	f()
}

// out prints one line of the current probe.
func out(format string, args ...any) { fmt.Printf(cur+": "+format+"\n", args...) }

// show prints a text line by line.
func show(title, text string) {
	for _, l := range strings.Split(text, "\n") {
		out("%s| %s", title, l)
	}
}

// Name is a named string type.
type Name string

// Safe is a program type over template.HTML: it is not template.HTML.
type Safe template.HTML

// Count is a named int type.
type Count int

// Money has a String method (value receiver).
type Money struct{ Cents int }

func (m Money) String() string { return fmt.Sprintf("$%d.%02d", m.Cents/100, m.Cents%100) }

// PMoney has a String method (pointer receiver).
type PMoney struct{ Cents int }

func (m *PMoney) String() string { return fmt.Sprintf("p$%d", m.Cents) }

// JSONVal has a MarshalJSON method (value receiver) and a String method.
type JSONVal struct{ V string }

func (j JSONVal) MarshalJSON() ([]byte, error) { return []byte(`{"json":"` + j.V + `"}`), nil }
func (j JSONVal) String() string               { return "string " + j.V }

// PJSON has a MarshalJSON method (pointer receiver).
type PJSON struct{ V int }

func (j *PJSON) MarshalJSON() ([]byte, error) { return []byte(fmt.Sprintf(`[%d]`, j.V)), nil }

// Plain has no method: it is encoded field by field.
type Plain struct {
	A Name
	B Count
	c string
	D []Name `json:"d,omitempty"`
}

// Gen is a generic type with a String method.
type Gen[T any] struct{ V T }

func (g Gen[T]) String() string { return fmt.Sprintf("gen<%v>", g.V) }

// Err is a program error type.
type Err struct{ Code int }

func (e *Err) Error() string { return fmt.Sprintf("<err %d>", e.Code) }

// Page is the data of the templates.
type Page struct {
	Title  Name
	Body   Safe
	HTML   template.HTML
	Price  Money
	PPrice *PMoney
	Val    PMoney
	J      JSONVal
	PJ     *PJSON
	VJ     PJSON
	Plain  Plain
	Gen    Gen[Count]
	Any    any
	Items  []Name
	Map    map[Name]Count
	Nil    *Plain
	Err    error
	URL    Name
	Attr   Name
}

func (p Page) Upper() Name { return Name(strings.ToUpper(string(p.Title))) }

func page() *Page {
	return &Page{
		Title:  "<b>Tom & Jerry</b>",
		Body:   "<i>body</i>",
		HTML:   "<i>trusted</i>",
		Price:  Money{1234},
		PPrice: &PMoney{5},
		Val:    PMoney{6},
		J:      JSONVal{"j"},
		PJ:     &PJSON{7},
		VJ:     PJSON{8},
		Plain:  Plain{"<a>", 3, "hidden", []Name{"x"}},
		Gen:    Gen[Count]{4},
		Any:    Money{99},
		Items:  []Name{"a<", "b>"},
		Map:    map[Name]Count{"k2": 2, "k1": 1},
		Err:    &Err{5},
		URL:    "javascript:alert(1)",
		Attr:   `x" onclick="y`,
	}
}

const htmlText = `<p title="{{.Title}}">{{.Title}} {{.Upper}}</p>
<div>{{.Body}} {{.HTML}} {{.Price}} {{.PPrice}} {{.Val}} {{.J}} {{.Plain}} {{.Gen}} {{.Any}} {{.Err}} {{.Nil}}</div>
<ul>{{range .Items}}<li>{{.}}</li>{{end}}</ul> {{range $k, $v := .Map}}{{$k}}={{$v}};{{end}}
<a href="{{.URL}}" data-x="{{.Attr}}">{{.Title | printf "%s!"}}</a> <a href="/q?x={{.Title}}">q</a>
<script>
var title = {{.Title}};
var price = {{.Price}};
var pprice = {{.PPrice}};
var val = {{.Val}};
var j = {{.J}};
var pj = {{.PJ}};
var vj = {{.VJ}};
var plain = {{.Plain}};
var gen = {{.Gen}};
var any = {{.Any}};
var items = {{.Items}};
var map = {{.Map}};
var nil = {{.Nil}};
var err = {{.Err}};
var str = "{{.Title}} {{.Price}}";
</script>
<style>p { color: {{.Title}} }</style>`

func main() {
	// row: html/template (*Template).Execute,(*Template).ExecuteTemplate * *
	probe("Execute", func() {
		t := template.Must(template.New("page").Parse(htmlText))
		var sb strings.Builder
		err := t.Execute(&sb, page())
		show("ptr", sb.String())
		out("err: %v", err)
		sb.Reset()
		err = t.ExecuteTemplate(&sb, "page", *page())
		show("value", sb.String())
		out("err: %v", err)
	})
	// row: html/template (*Template).Execute,(*Template).ExecuteTemplate assert x.(T), T unnamed
	probe("Execute/strings", func() {
		t := template.Must(template.New("s").Parse(`<p>{{.}}</p><a href="{{.}}">x</a><script>var s = {{.}};</script>`))
		for _, v := range []any{Name("<x>"), "<x>", Safe("<x>"), template.HTML("<x>"), template.JS("1+1"), Count(3), &PMoney{1}, []any{Name("a"), Count(1)}, map[string]any{"k": JSONVal{"m"}}} {
			var sb strings.Builder
			err := t.Execute(&sb, v)
			out("%T: %s %v", v, sb.String(), err)
		}
	})
	// row: html/template (*Template).Execute,(*Template).ExecuteTemplate assert x.(encoding/json.Marshaler)
	// row: html/template (*Template).Execute,(*Template).ExecuteTemplate assert x.(fmt.Stringer)
	// row: html/template (*Template).Execute,(*Template).ExecuteTemplate reflect method set
	probe("Execute/js", func() {
		t := template.Must(template.New("js").Parse(`<script>var v = {{.}};</script>`))
		var nilp *PJSON
		for _, v := range []any{JSONVal{"a"}, &JSONVal{"b"}, &PJSON{1}, PJSON{2}, nilp, Money{5}, &Money{6}, &PMoney{7}, PMoney{8}, Gen[Name]{"g"}, Plain{"p", 1, "", nil}, &Err{1}, Count(4), Name("n")} {
			var sb strings.Builder
			err := t.Execute(&sb, v)
			out("%T: %s %v", v, sb.String(), err)
		}
	})
}
