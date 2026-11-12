// Package hostmemaotplug writes to the globals of the AOT copies of the
// standard library (encoding/xml, database/sql), which all the plugins of
// the process share in AOT mode, without naming the variable in the write.
// "/write" tries the writes, "/read" shows what a plugin sees.
// TestHostMem loads two instances in AOT mode: the writes of the first
// must be refused, and the second must see the original values.
package hostmemaotplug

import (
	"context"
	"database/sql"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

// try runs f and says whether the VM refused it.
func try(w io.Writer, name string, f func()) {
	defer func() {
		if r := recover(); r != nil {
			if err, ok := r.(error); ok && strings.Contains(err.Error(), "writing to memory of the host") {
				fmt.Fprintf(w, "%s: refused\n", name)
				return
			}
			fmt.Fprintf(w, "%s: panic %v\n", name, r)
		}
	}()
	f()
	fmt.Fprintf(w, "%s: done\n", name)
}

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/read" {
			_, ok := xml.HTMLEntity["rv64"]
			fmt.Fprintf(w, "entity: %v\nerr: %s\n", ok, sql.ErrNoRows.Error())
			return
		}
		try(w, "xml map", func() {
			m := xml.HTMLEntity
			m["rv64"] = "x"
		})
		try(w, "sql error", func() {
			reflect.ValueOf(sql.ErrNoRows).Elem().Set(reflect.ValueOf(errors.New("pwned")).Elem())
		})
	}), nil
}
