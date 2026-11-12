// Plugin-like sample: what a small Traefik middleware typically touches.
package main

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

type Config struct{ Headers map[string]string }

type mw struct {
	next http.Handler
	cfg  *Config
}

func (m *mw) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	keys := make([]string, 0, len(m.cfg.Headers))
	for k := range m.cfg.Headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for i, k := range keys {
		req.Header.Set(strings.ToUpper(k), m.cfg.Headers[k]+strconv.Itoa(i))
	}
	if req.URL.Path == "/err" {
		http.Error(rw, fmt.Sprintf("%v", errors.New("boom")), http.StatusTeapot)
		return
	}
	m.next.ServeHTTP(rw, req)
}

func main() {
	h := &mw{next: http.NotFoundHandler(), cfg: &Config{Headers: map[string]string{"a": "b"}}}
	_ = http.ListenAndServe(":0", h)
}
