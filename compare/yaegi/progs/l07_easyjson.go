package main

import (
	"fmt"

	"github.com/mailru/easyjson/jlexer"
	"github.com/mailru/easyjson/jwriter"
)

func main() {
	l := jlexer.Lexer{Data: []byte(`{"a":"xé","b":12}`)}
	l.Delim('{')
	for !l.IsDelim('}') {
		k := l.UnsafeString()
		l.WantColon()
		switch k {
		case "a":
			fmt.Println("a=", l.String())
		case "b":
			fmt.Println("b=", l.Int())
		}
		l.WantComma()
	}
	l.Delim('}')
	fmt.Println(l.Error())
	var w jwriter.Writer
	w.RawByte('{')
	w.String("k")
	w.RawByte(':')
	w.Int(5)
	w.RawByte('}')
	b, _ := w.BuildBytes()
	fmt.Println(string(b))
}
