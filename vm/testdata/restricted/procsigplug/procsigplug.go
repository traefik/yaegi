// Package procsigplug signals a process that it builds itself
// (&os.Process{Pid: n}) without naming the methods of *os.Process: through
// an interface of its own and through its reflect. Each path must be
// refused when it runs, unless the host gives Options.Exec = ExecReal.
package procsigplug

import (
	"context"
	"net/http"
	"os"
	"reflect"
	"strconv"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type signaler interface{ Signal(os.Signal) error }

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pid, _ := strconv.Atoi(r.URL.Query().Get("pid"))
		var p any = &os.Process{Pid: pid}
		var err error
		switch r.URL.Path {
		case "/interface":
			err = p.(signaler).Signal(os.Kill)
		case "/reflect":
			out := reflect.ValueOf(p).MethodByName("Kill").Call(nil)
			err, _ = out[0].Interface().(error)
		}
		if err != nil {
			w.Write([]byte(err.Error()))
		}
	}), nil
}
