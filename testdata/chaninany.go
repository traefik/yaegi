// Interpreted channels given to native code in an interface:
// json, xml and gob see a channel and report their own error (or skip a
// field with `json:"-"`), and a channel stored by native code (a context,
// a sync.Map) comes back as the same channel. text/template and
// html/template receive from a channel and take its length: there the VM
// may give an error, but never a wrong result.
package main

import (
	"bytes"
	"context"
	"encoding/gob"
	"encoding/json"
	"encoding/xml"
	"fmt"
	htmltemplate "html/template"
	"io"
	"reflect"
	"strings"
	"sync"
	"text/template"
)

type withChan struct {
	A int
	C chan int
}

type skipped struct {
	A int
	C chan int `json:"-" xml:"-"`
}

type dirs struct {
	In  <-chan int
	Out chan<- int
}

type named chan string

func (n named) Name() string { return "named" }

type key int

func try(name string, f func()) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println(name, "panic:", r)
		}
	}()
	f()
}

func main() {
	ch := make(chan int, 3)

	try("json chan", func() { _, err := json.Marshal(ch); fmt.Println("json chan:", err) })
	try("json field", func() { _, err := json.Marshal(withChan{1, ch}); fmt.Println("json field:", err) })
	try("json skipped", func() { b, err := json.Marshal(skipped{1, ch}); fmt.Println("json skipped:", string(b), err) })
	try("json ptr", func() { _, err := json.Marshal(&withChan{1, ch}); fmt.Println("json ptr:", err) })
	try("json map any", func() { _, err := json.Marshal(map[string]any{"c": ch}); fmt.Println("json map any:", err) })
	try("json map chan", func() { _, err := json.Marshal(map[string]chan int{"c": ch}); fmt.Println("json map chan:", err) })
	try("json slice", func() { _, err := json.Marshal([]any{1, ch}); fmt.Println("json slice:", err) })
	try("json dirs", func() { _, err := json.Marshal(dirs{ch, ch}); fmt.Println("json dirs:", err) })
	try("json named", func() { _, err := json.Marshal(make(named)); fmt.Println("json named:", err != nil) })
	try("json nil chan", func() { _, err := json.Marshal(withChan{}); fmt.Println("json nil chan:", err) })

	try("xml chan", func() { _, err := xml.Marshal(ch); fmt.Println("xml chan:", err) })
	try("xml field", func() { _, err := xml.Marshal(withChan{1, ch}); fmt.Println("xml field:", err) })
	try("xml skipped", func() { b, err := xml.Marshal(skipped{1, ch}); fmt.Println("xml skipped:", string(b), err) })

	try("gob chan", func() {
		var buf bytes.Buffer
		fmt.Println("gob chan:", gob.NewEncoder(&buf).Encode(ch))
	})
	try("gob field", func() {
		// gob skips fields of type chan
		var buf bytes.Buffer
		err := gob.NewEncoder(&buf).Encode(withChan{1, ch})
		var out withChan
		if err == nil {
			err = gob.NewDecoder(&buf).Decode(&out)
		}
		fmt.Println("gob field:", err, out.A, out.C == nil)
	})

	try("fmt", func() {
		fmt.Printf("%T %T %T\n", ch, withChan{1, ch}, dirs{}.In)
		fmt.Println(fmt.Sprint(ch) != "", fmt.Sprintf("%v", withChan{1, nil}))
	})

	// Round trips: native code gives back the same channel.
	try("sync.Map", func() {
		var m sync.Map
		m.Store(ch, "by chan")
		v, ok := m.Load(ch)
		fmt.Println("sync.Map key:", v, ok)
		m.Store("dirs", dirs{In: ch, Out: ch})
		dv, _ := m.Load("dirs")
		d := dv.(dirs)
		d.Out <- 5
		fmt.Println("sync.Map value:", <-d.In, len(ch))
	})
	try("context", func() {
		n := make(named, 1)
		ctx := context.WithValue(context.Background(), key(1), n)
		got, ok := ctx.Value(key(1)).(named)
		fmt.Println("context:", ok, got == n, got.Name())
		got <- "sent"
		fmt.Println("context:", <-n)
		ctx = context.WithValue(ctx, key(2), &dirs{In: ch})
		ch <- 9
		fmt.Println("context ptr:", <-ctx.Value(key(2)).(*dirs).In)
		fmt.Printf("%T\n", ctx.Value(key(1)))
	})

	// Templates: go run prints "[1][2]" and "1". The VM gives an error in
	// default mode (it cannot give the values to native code), and the
	// right result in AOT mode. A stand-in (an empty closed channel) would
	// print "" and "0": a wrong result with no error.
	try("text/template", func() {
		c := make(chan int, 3)
		c <- 1
		c <- 2
		close(c)
		var b strings.Builder
		err := template.Must(template.New("x").Parse("{{range .C}}[{{.}}]{{end}}")).Execute(&b, withChan{1, c})
		fmt.Println("text/template range:", err != nil || b.String() == "[1][2]")
		c1 := make(chan int, 2)
		c1 <- 1
		b.Reset()
		err = template.Must(template.New("x").Parse("{{len .}}")).Execute(&b, c1)
		fmt.Println("text/template len:", err != nil || b.String() == "1")
		b.Reset()
		err = template.Must(template.New("x").Parse("{{len .C}}")).ExecuteTemplate(&b, "x", withChan{1, c1})
		fmt.Println("text/template len field:", err != nil || b.String() == "1")
	})
	try("html/template", func() {
		c := make(chan int, 3)
		c <- 1
		c <- 2
		close(c)
		var b strings.Builder
		err := htmltemplate.Must(htmltemplate.New("x").Parse("{{range .C}}[{{.}}]{{end}}")).Execute(&b, withChan{1, c})
		fmt.Println("html/template range:", err != nil || b.String() == "[1][2]")
		c1 := make(chan int, 2)
		c1 <- 1
		b.Reset()
		err = htmltemplate.Must(htmltemplate.New("x").Parse("{{len .}}")).ExecuteTemplate(&b, "x", c1)
		fmt.Println("html/template len:", err != nil || b.String() == "1")
	})
	// Callbacks of a template that return a channel in an any.
	try("template callbacks", func() {
		c := make(chan int, 2)
		c <- 1
		c <- 2
		close(c)
		var b strings.Builder
		t := template.New("x").Funcs(template.FuncMap{"get": func() any { return c }})
		err := template.Must(t.Parse("{{range get}}[{{.}}]{{end}}")).Execute(&b, nil)
		fmt.Println("text/template FuncMap:", err != nil || b.String() == "[1][2]")
		c2 := make(chan int, 2)
		c2 <- 1
		c2 <- 2
		close(c2)
		b.Reset()
		data := struct{ F func() any }{func() any { return c2 }}
		err = template.Must(template.New("x").Parse("{{range call .F}}[{{.}}]{{end}}")).Execute(&b, data)
		fmt.Println("text/template call:", err != nil || b.String() == "[1][2]")
		c3 := make(chan int, 2)
		c3 <- 1
		c3 <- 2
		close(c3)
		b.Reset()
		h := htmltemplate.New("x").Funcs(htmltemplate.FuncMap{"get": func() any { return c3 }})
		err = htmltemplate.Must(h.Parse("{{range get}}[{{.}}]{{end}}")).Execute(&b, nil)
		fmt.Println("html/template FuncMap:", err != nil || b.String() == "[1][2]")
	})
	// Execute called through reflect.
	try("template reflect", func() {
		mk := func() chan int {
			c := make(chan int, 2)
			c <- 1
			c <- 2
			close(c)
			return c
		}
		var b strings.Builder
		call := func(name string, t *template.Template, data any) {
			b.Reset()
			out := reflect.ValueOf(t).MethodByName("Execute").Call([]reflect.Value{
				reflect.ValueOf(io.Writer(&b)), reflect.ValueOf(&data).Elem()})
			err, _ := out[0].Interface().(error)
			fmt.Println(name, err != nil || b.String() == "[1][2]")
		}
		call("reflect field:", template.Must(template.New("x").Parse("{{range .C}}[{{.}}]{{end}}")), withChan{1, mk()})
		call("reflect chan:", template.Must(template.New("x").Parse("{{range .}}[{{.}}]{{end}}")), mk())
		c := mk()
		tf := template.Must(template.New("x").Funcs(template.FuncMap{"get": func() any { return c }}).Parse("{{range get}}[{{.}}]{{end}}"))
		call("reflect FuncMap:", tf, nil)

		c = mk() // the FuncMap call above read c
		b.Reset()
		f := reflect.ValueOf(tf).MethodByName("Execute").Interface().(func(io.Writer, any) error)
		err := f(&b, nil)
		fmt.Println("reflect method func:", err != nil || b.String() == "[1][2]")

		b.Reset()
		tr := template.Must(template.New("x").Parse("{{range .C}}[{{.}}]{{end}}"))
		m, _ := reflect.TypeOf(tr).MethodByName("Execute")
		var data any = withChan{1, mk()}
		out := m.Func.Call([]reflect.Value{reflect.ValueOf(tr), reflect.ValueOf(io.Writer(&b)), reflect.ValueOf(&data).Elem()})
		err, _ = out[0].Interface().(error)
		fmt.Println("reflect method expr:", err != nil || b.String() == "[1][2]")
	})
}
