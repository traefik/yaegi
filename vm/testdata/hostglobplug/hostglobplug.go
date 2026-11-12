// Package hostglobplug changes the variables os.Stdin, os.Stdout,
// os.Stderr and the settings of the default logger of the log package,
// then writes what it sees in the response. The host must keep its own.
// Before that, it prints with fmt and log: with no Stdout and no Stderr
// options, this goes to the host os.Stdout and to the writer of the host
// default logger. fmt.Scan reads the plugin's own os.Stdin.
package hostglobplug

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
		fmt.Println("fmt.Println", name)
		log.Println("log.Println", name)
		os.Stdout, os.Stderr = os.Stderr, os.Stdout
		pr, pw, err := os.Pipe()
		if err != nil {
			panic(err)
		}
		pw.WriteString("42\n")
		pw.Close()
		os.Stdin = pr
		var n int
		fmt.Scan(&n)
		pr.Close()
		os.Stdin = nil
		log.SetPrefix("PLUGPFX ")
		log.SetFlags(log.Lshortfile)
		log.SetOutput(io.Discard)
		log.Println("not printed")
		fmt.Fprint(w, n, " ", log.Prefix(), log.Flags() == log.Lshortfile, " ", log.Writer() == io.Discard, " ", os.Stdin == nil)
	}), nil
}
