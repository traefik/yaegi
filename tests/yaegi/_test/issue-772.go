package main

import (
	"log"
	"os"
	"text/template"
)

type Message struct {
	Data string
}

func main() {
	tmpl := template.New("name")

	_, err := tmpl.Parse("{{.Data}}")
	if err != nil {
		log.Fatal(err)
	}

	err = tmpl.Execute(os.Stdout, Message{
		Data: "Hello, World!!",
	})

	if err != nil {
		log.Fatal(err)
	}
}

// Output:
// Hello, World!!
