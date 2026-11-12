package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
)

// Native calls with a fast trampoline, given interpreted implementations of
// their interface parameters.

type upperReader struct{ r io.Reader }

func (u upperReader) Read(p []byte) (int, error) {
	n, err := u.r.Read(p)
	copy(p, strings.ToUpper(string(p[:n])))
	return n, err
}

type recorder struct {
	h    http.Header
	code int
	body strings.Builder
}

func (r *recorder) Header() http.Header         { return r.h }
func (r *recorder) Write(b []byte) (int, error) { return r.body.Write(b) }
func (r *recorder) WriteHeader(code int)        { r.code = code }

// countingWriter wraps a native ResponseWriter, as body-rewriting and
// logging plugins do before calling the next (native) handler.
type countingWriter struct {
	http.ResponseWriter
	n int
}

func (c *countingWriter) Write(b []byte) (int, error) {
	c.n += len(b)
	return c.ResponseWriter.Write(b)
}

func main() {
	b, err := io.ReadAll(upperReader{strings.NewReader("hello")})
	fmt.Println(string(b), err)

	rec := &recorder{h: http.Header{}}
	http.Error(rec, "nope", http.StatusTeapot)
	fmt.Printf("%d %q %q\n", rec.code, rec.body.String(), rec.h.Get("Content-Type"))

	// A native handler's ServeHTTP called with interpreted ResponseWriters.
	var next http.Handler = http.NotFoundHandler()
	req := httptest.NewRequest("GET", "/x", nil)
	rec = &recorder{h: http.Header{}}
	next.ServeHTTP(rec, req)
	fmt.Printf("%d %q\n", rec.code, rec.body.String())
	w := httptest.NewRecorder()
	cw := &countingWriter{ResponseWriter: w}
	next.ServeHTTP(cw, req)
	fmt.Println(w.Code, cw.n, strings.TrimSpace(w.Body.String()))
}
