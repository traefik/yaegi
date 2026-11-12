package gojaplug

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/dop251/goja"
)

// state runs a JavaScript handler (ES2015+) with goja, a JS engine written
// in Go: a JS engine inside the Go interpreter. The script sees a Go object
// and calls Go functions.
type state struct {
	mu  sync.Mutex
	vm  *goja.Runtime
	fn  goja.Callable
	log []string
}

const script = `
class Router {
  constructor() { this.routes = new Map(); this.count = 0; }
  add(prefix, fn) { this.routes.set(prefix, fn); return this; }
  handle(req) {
    this.count++;
    for (const [prefix, fn] of this.routes) {
      if (req.path.startsWith(prefix)) return fn(req, this.count);
    }
    return { action: "pass", n: this.count };
  }
}
const router = new Router()
  .add("/api/", (req, n) => ({ action: "json", n, body: JSON.parse(req.body || "{}") }))
  .add("/admin", (req, n) => { goLog("deny " + req.host); return { action: "deny", n }; })
  .add("/path/", (req, n) => ({ action: "rewrite", n, to: req.path.split("/").filter(Boolean).reverse().join("/") }));

function handle(req) {
  const r = router.handle(req);
  const hdrs = Object.keys(req.headers).sort().map(k => k.toLowerCase());
  return JSON.stringify({ ...r, hdrs, sum: [1, 2, 3].reduce((a, b) => a + b, 0), up: goUpper(req.method) });
}
`

// Request is the Go value given to the script.
type Request struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Host    string            `json:"host"`
	Body    string            `json:"body"`
	Headers map[string]string `json:"headers"`
}

func setup(_ context.Context) (*state, error) {
	s := &state{vm: goja.New()}
	s.vm.SetFieldNameMapper(goja.TagFieldNameMapper("json", true))
	s.vm.Set("goUpper", strings.ToUpper)
	s.vm.Set("goLog", func(msg string) { s.log = append(s.log, msg) })
	if _, err := s.vm.RunString(script); err != nil {
		return nil, err
	}
	fn, ok := goja.AssertFunction(s.vm.Get("handle"))
	if !ok {
		return nil, fmt.Errorf("handle is not a function")
	}
	s.fn = fn
	// A JS exception becomes a Go error.
	if _, err := s.vm.RunString(`null.x`); err == nil {
		return nil, fmt.Errorf("no exception")
	}
	return s, nil
}

func (s *state) process(req *http.Request) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := &Request{Method: req.Method, Path: req.URL.Path, Host: req.Host, Headers: map[string]string{}}
	for k := range req.Header {
		r.Headers[k] = req.Header.Get(k)
	}
	if req.Method == http.MethodPost {
		r.Body = `{"name":"test","value":42}`
	}
	v, err := s.fn(goja.Undefined(), s.vm.ToValue(r))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s log=%v", v.String(), s.log), nil
}
