//go:build goexperiment.jsonv2

package main

// The generic functions of encoding/json/jsontext (Go 1.27) run for every
// type argument: AppendQuote, AppendUnquote and
// AppendFormat take a ~[]byte or a ~string. The OPA SDK calls
// jsontext.AppendQuote[[]byte]; the VM refused them at load time.

import (
	"encoding/json/jsontext"
	"fmt"
)

type name string

type raw []byte

func main() {
	b, err := jsontext.AppendQuote([]byte("x="), `say "hi"`)
	fmt.Println(string(b), err)
	b, err = jsontext.AppendQuote(nil, []byte("tab\there"))
	fmt.Println(string(b), err)
	b, err = jsontext.AppendQuote([]byte("n="), name("named <string>"))
	fmt.Println(string(b), err)
	b, err = jsontext.AppendQuote(nil, raw("bad \xff utf-8"))
	fmt.Println(string(b), err != nil)

	b, err = jsontext.AppendUnquote([]byte("u="), `"aéb\n"`)
	fmt.Printf("%q %v\n", b, err)
	b, err = jsontext.AppendUnquote(nil, raw(`"named"`))
	fmt.Printf("%q %v\n", b, err)
	_, err = jsontext.AppendUnquote(nil, "not quoted")
	fmt.Println(err != nil)

	b, err = jsontext.AppendFormat(nil, `{ "a" : [1, 2] }`)
	fmt.Println(string(b), err)
	b, err = jsontext.AppendFormat([]byte("> "), raw(`{"a":[1,2]}`), jsontext.Multiline(true), jsontext.WithIndent("  "))
	fmt.Println(string(b), err)
	b, err = jsontext.AppendFormat(nil, name(`{"a":1,"a":2}`))
	fmt.Println(string(b), err != nil)
}
