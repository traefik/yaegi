// Package hostvarsplug changes the default client, transport, mux and
// resolver of net/http and net, the default logger of log/slog, and
// GOMAXPROCS, as a plugin may try to.
// The host must keep its own. The handler then uses its copies: it fetches
// the URL of the X-Target header with http.Get, with a method value of its
// DefaultClient, and with a new client that has no transport (that one uses
// the plugin's DefaultTransport too, as in Go), and serves its own mux.
package hostvarsplug

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"runtime"
	"strings"
	"time"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

func New(_ context.Context, _ http.Handler, _ *Config, name string) (http.Handler, error) {
	if name == "insecure" {
		// The plugin turns off the TLS verification of its default
		// transport only.
		http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	http.DefaultClient.Timeout = 7 * time.Second
	net.DefaultResolver.PreferGo = !net.DefaultResolver.PreferGo
	http.HandleFunc("/evil", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, "evil ", name) })
	http.DefaultServeMux.Handle("/evil2", http.NotFoundHandler())
	procs := runtime.GOMAXPROCS(1)
	var logs strings.Builder
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	})))
	slog.Info("new", "name", name)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slog" {
			fmt.Fprint(w, logs.String())
			return
		}
		if r.URL.Path == "/evil" {
			http.DefaultServeMux.ServeHTTP(w, r)
			return
		}
		target := r.Header.Get("X-Target")
		get := (&http.Client{}).Get
		for _, f := range []func(string) (*http.Response, error){http.Get, get, http.DefaultClient.Get} {
			resp, err := f(target)
			if err != nil {
				fmt.Fprint(w, "error ")
				continue
			}
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			fmt.Fprintf(w, "%s ", b)
		}
		fmt.Fprint(w, procs == runtime.GOMAXPROCS(0), " ", http.DefaultClient)
	}), nil
}
