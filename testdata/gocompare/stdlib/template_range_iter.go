// text/template and html/template range over an iter.Seq or iter.Seq2 of
// the program, and reflect.Value.Seq and Seq2 of a func of the program.
package main

import (
	"fmt"
	htmltemplate "html/template"
	"iter"
	"maps"
	"os"
	"reflect"
	"slices"
	"text/template"
)

type Ints func(yield func(int) bool)

func count(n int) iter.Seq[int] {
	return func(yield func(int) bool) {
		for i := range n {
			if !yield(i * 10) {
				return
			}
		}
	}
}

func pairs() iter.Seq2[string, int] {
	return func(yield func(string, int) bool) {
		_ = yield("a", 1) && yield("b", 2)
	}
}

func main() {
	one := template.Must(template.New("").Parse("{{range .}}{{.}} {{end}}\n"))
	two := template.Must(template.New("").Parse("{{range $k, $v := .}}{{$k}}={{$v}} {{end}}\n"))
	stop := template.Must(template.New("").Parse("{{range .}}{{if eq . 10}}{{break}}{{end}}{{.}} {{else}}empty{{end}}\n"))
	for _, x := range []any{count(3), slices.Values([]int{4, 5}), Ints(func(yield func(int) bool) { yield(7) })} {
		if err := one.Execute(os.Stdout, x); err != nil {
			panic(err)
		}
	}
	two.Execute(os.Stdout, pairs())
	two.Execute(os.Stdout, maps.All(map[string]int{"z": 9}))
	stop.Execute(os.Stdout, count(5))
	stop.Execute(os.Stdout, count(0))
	// The error message holds the address of the func.
	fmt.Println(two.Execute(os.Stdout, count(2)) != nil)
	h := htmltemplate.Must(htmltemplate.New("").Parse("{{range .}}<b>{{.}}</b>{{end}}\n"))
	h.Execute(os.Stdout, count(2))

	v := reflect.ValueOf(count(4))
	fmt.Println(v.Type().CanSeq(), v.Type().CanSeq2(), reflect.TypeOf(pairs()).CanSeq2(), reflect.TypeOf(func() {}).CanSeq())
	for x := range v.Seq() {
		if x.Int() == 20 {
			break
		}
		fmt.Println(x.Int())
	}
	for k, x := range reflect.ValueOf(pairs()).Seq2() {
		fmt.Println(k.String(), x.Int())
	}
	// a variadic yield cannot range: gc counts the variadic bit with the
	// results
	fmt.Println(reflect.TypeOf(varYield(nil)).CanSeq(), reflect.TypeOf(varYield2(nil)).CanSeq2())
	func() {
		defer func() { fmt.Println(recover()) }()
		reflect.ValueOf(varYield(func(y func(...int) bool) { y(1, 2) })).Seq()
	}()
}

type varYield func(func(...int) bool)

type varYield2 func(func(int, ...string) bool)
