package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
)

type Config struct{ Header string }

func CreateConfig() *Config { return &Config{Header: "X-Plugin"} }

type Plugin struct {
	next http.Handler
	cfg  *Config
	name string
}

func New(_ context.Context, next http.Handler, cfg *Config, name string) (http.Handler, error) {
	return &Plugin{next: next, cfg: cfg, name: name}, nil
}

func (p *Plugin) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	req.Header.Set(p.cfg.Header, p.name)
	rw.Header().Set("X-Seen", req.URL.Path)
	p.next.ServeHTTP(&recorder{ResponseWriter: rw}, req)
}

type recorder struct {
	http.ResponseWriter
	code int
}

func (r *recorder) WriteHeader(code int) {
	r.code = code
	r.ResponseWriter.Header().Set("X-Code", fmt.Sprint(code))
	r.ResponseWriter.WriteHeader(code)
}

func main() {
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusTeapot)
		fmt.Fprintf(rw, "hdr=%s", req.Header.Get("X-Plugin"))
	})
	h, _ := New(context.Background(), next, CreateConfig(), "demo")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/path", nil))
	b, _ := io.ReadAll(rec.Body)
	fmt.Println(rec.Code, rec.Header().Get("X-Seen"), rec.Header().Get("X-Code"), string(b))
	srv := httptest.NewServer(h)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/real")
	if err != nil {
		panic(err)
	}
	b, _ = io.ReadAll(resp.Body)
	fmt.Println(resp.StatusCode, resp.Header.Get("X-Seen"), string(b))
	_, isFlusher := any(&recorder{}).(http.Flusher)
	fmt.Println(isFlusher)
}
