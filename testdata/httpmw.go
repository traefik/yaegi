// net/http with interpreted handlers, as a Traefik plugin does: a chain of
// middlewares, an interpreted ResponseWriter wrapper that records the
// status and keeps http.Flusher through http.ResponseController, header
// changes before and after WriteHeader, http.Error, http.StripPrefix and a
// ServeMux with patterns, request cloning with a context value, cookies,
// query parsing, and a round trip through httptest.NewServer.
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

type ctxKey struct{}

var log []string

func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		log = append(log, fmt.Sprintf("%s %s -> %d (%d bytes) user=%v", r.Method, r.URL.Path, rec.status, rec.bytes, r.Context().Value(ctxKey{})))
	})
}

func auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := r.Header.Get("X-User")
		if user == "" {
			http.Error(w, "no user", http.StatusUnauthorized)
			return
		}
		r2 := r.Clone(context.WithValue(r.Context(), ctxKey{}, user))
		r2.Header.Del("X-User")
		r2.Header.Set("X-Auth", "ok:"+user)
		next.ServeHTTP(w, r2)
	})
}

func headers(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Before", "1")
		next.ServeHTTP(w, r)
		w.Header().Set("X-After", "ignored after write")
	})
}

func app() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /hello/{name}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "hello %s (auth=%s, q=%v)", r.PathValue("name"), r.Header.Get("X-Auth"), r.URL.Query()["q"])
	})
	mux.HandleFunc("POST /echo", func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8))
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			http.Error(w, fmt.Sprint("too large, limit ", mbe.Limit), http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusCreated)
		w.Write(b)
	})
	mux.HandleFunc("/stream", func(w http.ResponseWriter, r *http.Request) {
		rc := http.NewResponseController(w)
		for i := range 3 {
			fmt.Fprintf(w, "chunk%d;", i)
			if err := rc.Flush(); err != nil {
				fmt.Fprint(w, "flush error: ", err)
				return
			}
		}
	})
	mux.HandleFunc("/cookie", func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("session")
		if err != nil {
			fmt.Fprint(w, "no cookie: ", err)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "seen", Value: c.Value + "-2", Path: "/", HttpOnly: true})
		fmt.Fprint(w, "cookie ", c.Value)
	})
	mux.Handle("/static/", http.StripPrefix("/static", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "file ", r.URL.Path)
	})))
	mux.HandleFunc("/redirect", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/hello/redirected", http.StatusFound)
	})
	return mux
}

func show(w *httptest.ResponseRecorder) {
	res := w.Result()
	var hk []string
	for k, v := range res.Header {
		if k == "Date" {
			continue
		}
		hk = append(hk, k+"="+strings.Join(v, ","))
	}
	sort.Strings(hk)
	body, _ := io.ReadAll(res.Body)
	fmt.Printf("%d %q %v flushed=%v\n", res.StatusCode, body, hk, w.Flushed)
}

func main() {
	h := logging(headers(auth(app())))
	do := func(method, target, user, body string, cookies ...*http.Cookie) {
		var br io.Reader
		if body != "" {
			br = strings.NewReader(body)
		}
		r := httptest.NewRequest(method, target, br)
		if user != "" {
			r.Header.Set("X-User", user)
		}
		for _, c := range cookies {
			r.AddCookie(c)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		show(w)
	}
	do("GET", "/hello/bob?q=1&q=2", "alice", "")
	do("GET", "/hello/bob", "", "")
	do("POST", "/echo", "u", "short")
	do("POST", "/echo", "u", "this body is too long")
	do("GET", "/echo", "u", "")
	do("GET", "/stream", "u", "")
	do("GET", "/cookie", "u", "", &http.Cookie{Name: "session", Value: "abc"})
	do("GET", "/cookie", "u", "")
	do("GET", "/static/css/site.css", "u", "")
	do("GET", "/redirect", "u", "")
	do("GET", "/missing", "u", "")
	for _, l := range log {
		fmt.Println(l)
	}

	// A real round trip through a local server.
	srv := httptest.NewServer(logging(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Proto", r.Proto)
		if r.URL.Path == "/hijack" {
			hj, ok := w.(http.Hijacker)
			if !ok {
				http.Error(w, "no hijack", 500)
				return
			}
			conn, buf, err := hj.Hijack()
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			defer conn.Close()
			buf.WriteString("HTTP/1.1 200 OK\r\nContent-Length: 8\r\nConnection: close\r\n\r\nhijacked")
			buf.Flush()
			return
		}
		fmt.Fprintf(w, "served %s %s", r.Method, r.URL.RequestURI())
	})))
	defer srv.Close()
	log = nil
	res, err := http.Get(srv.URL + "/path?x=1")
	if err != nil {
		fmt.Println(err)
		return
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	fmt.Println(res.StatusCode, string(b), res.Header.Get("X-Proto"))
	res, err = http.Get(srv.URL + "/hijack")
	if err != nil {
		fmt.Println(err)
		return
	}
	b, _ = io.ReadAll(bufio.NewReader(res.Body))
	res.Body.Close()
	fmt.Println(res.StatusCode, string(b))
	srv.Close()
	fmt.Println(log[0])
}
