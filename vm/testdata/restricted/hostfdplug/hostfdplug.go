// Package hostfdplug makes one file call per request, with the arguments of
// the query (op, path, dir, name), and answers "ok" and the first bytes it
// read, or the error. TestHostFD (vm/hostfd_test.go) gives the paths.
package hostfdplug

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

func answer(w io.Writer, r io.ReadCloser, err error) {
	if err != nil {
		fmt.Fprint(w, err)
		return
	}
	defer r.Close()
	b, _ := io.ReadAll(io.LimitReader(r, 16))
	fmt.Fprintf(w, "ok %q", b)
}

// serve answers with the body of the handler h for a GET of name: "ok" and
// the first bytes on a 200, the status and the body otherwise.
func serve(w io.Writer, h http.Handler, name string) {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "http://x"+name, nil))
	if rec.Code != 200 {
		fmt.Fprintf(w, "%d %s", rec.Code, rec.Body.String())
		return
	}
	b, _ := io.ReadAll(io.LimitReader(rec.Body, 16))
	fmt.Fprintf(w, "ok %q", b)
}

// embedDir is a type of the plugin with the Open method of http.Dir.
type embedDir struct{ http.Dir }

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		p, dir, name := q.Get("path"), q.Get("dir"), q.Get("name")
		switch q.Get("op") {
		case "open":
			f, err := os.Open(p)
			answer(w, f, err)
		case "openfile":
			f, err := os.OpenFile(p, os.O_RDWR, 0)
			answer(w, f, err)
		case "readfile":
			b, err := os.ReadFile(p)
			if err != nil {
				fmt.Fprint(w, err)
				return
			}
			fmt.Fprintf(w, "ok %q", b)
		case "writefile":
			fmt.Fprint(w, os.WriteFile(p, []byte("plugin"), 0o644))
		case "create":
			f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE, 0o644)
			if err == nil {
				f.Close()
			}
			fmt.Fprint(w, err)
		case "createexcl":
			f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
			if err == nil {
				f.Close()
			}
			fmt.Fprint(w, err)
		case "dirfs":
			f, err := os.DirFS(dir).Open(name)
			if err != nil {
				fmt.Fprint(w, err)
				return
			}
			answer(w, f, nil)
		case "root":
			root, err := os.OpenRoot(dir)
			if err != nil {
				fmt.Fprint(w, err)
				return
			}
			defer root.Close()
			f, err := root.Open(name)
			answer(w, f, err)
		case "httpdir":
			f, err := http.Dir(dir).Open(name)
			answer(w, f, err)
		case "ifacedir":
			var fsys http.FileSystem = http.Dir(dir)
			f, err := fsys.Open(name)
			answer(w, f, err)
		case "fileserver":
			serve(w, http.FileServer(http.Dir(dir)), name)
		case "embeddir":
			serve(w, http.FileServer(embedDir{http.Dir(dir)}), name)
		case "servefile":
			serve(w, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.ServeFile(w, r, p)
			}), "/")
		case "filetransport":
			resp, err := http.NewFileTransport(http.Dir(dir)).RoundTrip(httptest.NewRequest("GET", "file://x"+name, nil))
			if err != nil {
				fmt.Fprint(w, err)
				return
			}
			if resp.StatusCode != 200 {
				fmt.Fprint(w, resp.Status)
				return
			}
			answer(w, resp.Body, nil)
		case "openinroot":
			f, err := os.OpenInRoot(dir, name)
			answer(w, f, err)
		case "filelistener":
			f, err := os.Open(p)
			if err != nil {
				f = os.Stdin
			}
			_, err = net.FileListener(f)
			fmt.Fprint(w, err)
		case "fileconn":
			_, err := net.FileConn(os.Stdin)
			fmt.Fprint(w, err)
		case "writestd":
			// As a logging plugin with "file: /dev/stdout".
			f, err := os.OpenFile(p, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
			if err != nil {
				fmt.Fprint(w, err)
				return
			}
			_, err = fmt.Fprintf(f, "line of %s\n", p)
			fmt.Fprint(w, "ok ", err, " ", f.Close())
		case "closestdio":
			fmt.Fprint(w, os.Stdout.Close(), " ", os.Stderr.Close(), " ", os.Stdin.Close())
		}
	}), nil
}
