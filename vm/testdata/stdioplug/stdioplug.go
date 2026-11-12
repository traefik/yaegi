// Package stdioplug writes to its standard output and standard error in
// all the ways a plugin can (fmt, os.Stdout, os.Stderr, the log package,
// the print and println builtins), and panics with http.ErrAbortHandler
// on the path /abort and with a string on the path /boom.
package stdioplug

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

func New(_ context.Context, _ http.Handler, _ *Config, name string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/abort" {
			io.WriteString(w, "partial")
			panic(http.ErrAbortHandler)
		}
		if r.URL.Path == "/boom" {
			panic("boom")
		}
		fmt.Println("fmt.Println", name)
		fmt.Printf("fmt.Printf %s\n", name)
		fmt.Print("fmt.Print ", name, "\n")
		os.Stdout.Write([]byte("os.Stdout.Write " + name + "\n"))
		fmt.Fprintln(os.Stderr, "fmt.Fprintln(os.Stderr)", name)
		log.SetFlags(0)
		log.Println("log.Println", name)
		log.Default().Printf("log.Default %s", name)
		fmt.Fprintf(log.Writer(), "log.Writer %s\n", name)
		println("println", name)
		func() {
			defer func() { recover() }()
			log.Fatal("log.Fatal ", name)
		}()
	}), nil
}
