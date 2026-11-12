// //go:embed of a file into a string and a []byte: protobuf embeds its
// edition defaults (internal/editiondefaults).
package main

import (
	_ "embed"
	"fmt"
	"strings"
)

//go:embed goembed.txt
var text string

//go:embed goembed.txt
var data []byte

func main() {
	fmt.Printf("%q\n", text)
	fmt.Println(len(data), string(data) == text, strings.Count(text, "\n"))
	data[0] = 'E' // a []byte is a variable of its own
	fmt.Println(text[:1], string(data[:1]))
}
