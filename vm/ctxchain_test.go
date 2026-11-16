package vm

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// ctxSetSrc is plugin A: it stores values of many kinds in the context of
// the request, as auth plugins pass claims and user ids to the next
// middleware and to the host.
const ctxSetSrc = `package p

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type User struct{ Name string }

// holder returns a value of A as any: B gets the same *User, with its
// String method.
type holder struct {
	u *User
	t *tagged
}

func (h *holder) Get() any  { return h.u }
func (h *holder) GetT() any { return h.t }

func (u *User) String() string { return "user:" + u.Name }

type myErr struct{ msg string }

func (e *myErr) Error() string { return "err:" + e.msg }

type tagged struct{ id int }

func (t *tagged) Tag() string { return fmt.Sprint("tag", t.id) }

type handler struct{ next http.Handler }

func New(ctx context.Context, next http.Handler, cfg *Config, name string) (http.Handler, error) {
	return handler{next}, nil
}

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("X-A-local", fmt.Sprintf("%T", ctx.Value(http.LocalAddrContextKey)))
	w.Header().Set("X-A-server", fmt.Sprintf("%T", ctx.Value(http.ServerContextKey)))
	ctx = context.WithValue(ctx, "s", "hello")
	ctx = context.WithValue(ctx, "sl", []string{"a", "b"})
	ctx = context.WithValue(ctx, "ms", map[string]string{"k": "v"})
	ctx = context.WithValue(ctx, "ma", map[string]any{"n": 1, "s": "x"})
	ctx = context.WithValue(ctx, "st", struct{ ID int }{7})
	ctx = context.WithValue(ctx, "fn", func(s string) string { return "fn:" + s })
	ctx = context.WithValue(ctx, "err", error(&myErr{"bad"}))
	ctx = context.WithValue(ctx, "user", &User{"bob"})
	ctx = context.WithValue(ctx, "tag", &tagged{3})
	ctx = context.WithValue(ctx, time.Duration(3), "dur")
	ctx = context.WithValue(ctx, http.LocalAddrContextKey, "fromA")
	// JWT claims: slices and maps nested in a map[string]any
	var claims map[string]any
	json.Unmarshal([]byte(` + "`" + `{"sub":"x","groups":["g1","g2"],"o":{"k":"v"}}` + "`" + `), &claims)
	ctx = context.WithValue(ctx, "jwt", claims)
	ctx = context.WithValue(ctx, "mx", map[string]any{
		"roles": []string{"r1", "r2"},
		"m":     map[string]string{"k": "v"},
		"n":     []map[string]any{{"a": 1}},
		"p":     &User{"nested"},
	})
	// values of native named types
	ctx = context.WithValue(ctx, "time", time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC))
	ctx = context.WithValue(ctx, "d", 3*time.Second)
	u, _ := url.Parse("http://h/p?q=1")
	ctx = context.WithValue(ctx, "url", u)
	ctx = context.WithValue(ctx, "ip", net.ParseIP("10.0.0.1"))
	ctx = context.WithValue(ctx, "cookie", &http.Cookie{Name: "c", Value: "cv"})
	ctx = context.WithValue(ctx, "num", json.Number("12"))
	ctx = context.WithValue(ctx, "hdr", http.Header{"K": {"hv"}})
	ctx = context.WithValue(ctx, "vals", url.Values{"q": {"1"}})
	ctx = context.WithValue(ctx, "req", r)
	sm := &sync.Map{}
	sm.Store("k", "smv")
	ctx = context.WithValue(ctx, "smap", sm)
	av := &atomic.Value{}
	av.Store("avv")
	ctx = context.WithValue(ctx, "aval", av)
	ctx = context.WithValue(ctx, "buf", bytes.NewBufferString("bufv"))
	sb := &strings.Builder{}
	sb.WriteString("sbv")
	ctx = context.WithValue(ctx, "sb", sb)
	ctx = context.WithValue(ctx, "raw", json.RawMessage(` + "`" + `{"a":1}` + "`" + `))
	ctx = context.WithValue(ctx, "holder", &holder{&User{"ann"}, &tagged{5}})
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	ctx = context.WithValue(ctx, "late", "after-cancel")
	// A reads its own slice through the native cancel context: the same
	// slice, as in Go
	sl := ctx.Value("sl").([]string)
	sl[0] = "A"
	w.Header().Set("X-A-sl", fmt.Sprint(ctx.Value("sl")))
	h.next.ServeHTTP(w, r.WithContext(ctx))
}
`

// ctxGetSrc is plugin B: it reads the values of A in the context and
// reports what it finds in the response headers.
const ctxGetSrc = `package p

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type handler struct{ next http.Handler }

func New(ctx context.Context, next http.Handler, cfg *Config, name string) (http.Handler, error) {
	return handler{next}, nil
}

// pass wraps a context, as middleware libraries do.
type pass struct{ context.Context }

type sk struct{}

type ik int

var pk = new(int)

` + ctxCheckSrc + `

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	set := func(k string, v ...any) { w.Header().Set("X-B-"+k, fmt.Sprint(v...)) }
	ctxCheck(ctx, set)
	// keys of named, pointer and struct types through a context wrapper
	c := context.WithValue(context.Background(), sk{}, "struct")
	c = context.WithValue(c, ik(1), "int")
	c = context.WithValue(c, pk, "ptr")
	c = context.WithValue(c, "str", "s")
	w1 := context.WithValue(pass{c}, "o", 1)
	w2, cancel := context.WithCancel(pass{c})
	defer cancel()
	set("pass", w1.Value(sk{}), w1.Value(ik(1)), w1.Value(pk), w1.Value("str"), pass{c}.Value(pk))
	set("pass-cancel", w2.Value(sk{}), w2.Value(ik(1)), w2.Value(pk), w2.Value("str"))
	// a value of A returned as any by a method of A
	if g, ok := ctx.Value("holder").(interface{ Get() any }); ok {
		a, b := g.Get(), g.Get()
		s, isS := a.(fmt.Stringer)
		set("holder", a == b, isS, isS && s.String() == "user:ann")
	} else {
		set("holder", "false")
	}
	if g, ok := ctx.Value("holder").(interface{ GetT() any }); ok {
		a := g.GetT()
		t, isT := a.(interface{ Tag() string })
		set("holder-t", a == g.GetT(), isT, isT && t.Tag() == "tag5")
	} else {
		set("holder-t", "false")
	}
	// B changes shared values: the host sees the changes
	if hd, ok := ctx.Value("hdr").(http.Header); ok {
		hd.Set("B", "1")
	}
	if sm, ok := ctx.Value("smap").(*sync.Map); ok {
		sm.Store("b", "1")
	}
	if ck, ok := ctx.Value("cookie").(*http.Cookie); ok {
		ck.Path = "/b"
	}
	if u, ok := ctx.Value("url").(*url.URL); ok {
		u.Host = "b"
	}
	set("s", ctx.Value("s"))
	set("late", ctx.Value("late"))
	sl, ok := ctx.Value("sl").([]string)
	set("sl", ok, sl)
	ms, ok := ctx.Value("ms").(map[string]string)
	set("ms", ok, ms)
	ma, ok := ctx.Value("ma").(map[string]any)
	set("ma", ok, ma)
	st, ok := ctx.Value("st").(struct{ ID int })
	set("st", ok, st)
	if fn, ok := ctx.Value("fn").(func(string) string); ok {
		set("fn", fn("x"))
	} else {
		set("fn", "false")
	}
	if e, ok := ctx.Value("err").(error); ok {
		set("err", e.Error())
	} else {
		set("err", "false")
	}
	if u, ok := ctx.Value("user").(fmt.Stringer); ok {
		set("user", u.String())
	} else {
		set("user", "false")
	}
	if t, ok := ctx.Value("tag").(interface{ Tag() string }); ok {
		set("tag", t.Tag())
	} else {
		set("tag", "false")
	}
	_, ok = ctx.Value("tag").(fmt.Stringer)
	set("tag-stringer", ok)
	set("dur", ctx.Value(time.Duration(3)))
	set("local", ctx.Value(http.LocalAddrContextKey))
	set("server", fmt.Sprintf("%T", ctx.Value(http.ServerContextKey)))
	h.next.ServeHTTP(w, r)
}
`

// ctxHostHandler is the host handler after the plugins.
func ctxHostHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	set := func(k string, v ...any) { w.Header().Set("X-H-"+k, fmt.Sprint(v...)) }
	ctxCheck(ctx, set)
	if hd, ok := ctx.Value("hdr").(http.Header); ok {
		set("hdr-b", hd.Get("B"))
	}
	if sm, ok := ctx.Value("smap").(*sync.Map); ok {
		v, _ := sm.Load("b")
		set("smap-b", v)
	}
	if ck, ok := ctx.Value("cookie").(*http.Cookie); ok {
		set("cookie-b", ck.Path)
	}
	if u, ok := ctx.Value("url").(*url.URL); ok {
		set("url-b", u.Host)
	}
	set("s", ctx.Value("s"))
	sl, ok := ctx.Value("sl").([]string)
	set("sl", ok, sl)
	ms, ok := ctx.Value("ms").(map[string]string)
	set("ms", ok, ms)
	ma, ok := ctx.Value("ma").(map[string]any)
	set("ma", ok, ma)
	if fn, ok := ctx.Value("fn").(func(string) string); ok {
		set("fn", fn("x"))
	} else {
		set("fn", "false")
	}
	if e, ok := ctx.Value("err").(error); ok {
		set("err", e.Error())
	} else {
		set("err", "false")
	}
	if u, ok := ctx.Value("user").(fmt.Stringer); ok {
		set("user", u.String())
	} else {
		set("user", "false")
	}
	set("dur", ctx.Value(time.Duration(3)))
	set("local", ctx.Value(http.LocalAddrContextKey))
}

// ctxWant is what the same code compiled with Go gives (the plugins as
// two packages of one program, behind a real server), except for
// http.ServerContextKey: a restricted plugin reads nil for it (keyNative),
// an unrestricted one reads the *http.Server as in Go (ctxWantUnrestricted).
var ctxWant = map[string]string{
	"X-A-Local":        "*net.TCPAddr",
	"X-A-Server":       "<nil>",
	"X-A-Sl":           "[A b]",
	"X-B-S":            "hello",
	"X-B-Late":         "after-cancel",
	"X-B-Sl":           "true [A b]",
	"X-B-Ms":           "true map[k:v]",
	"X-B-Ma":           "true map[n:1 s:x]",
	"X-B-St":           "true {7}",
	"X-B-Fn":           "fn:x",
	"X-B-Err":          "err:bad",
	"X-B-User":         "user:bob",
	"X-B-Tag":          "tag3",
	"X-B-Tag-Stringer": "false",
	"X-B-Dur":          "dur",
	"X-B-Local":        "fromA",
	"X-B-Server":       "<nil>",
	"X-H-S":            "hello",
	"X-H-Sl":           "true [A b]",
	"X-H-Ms":           "true map[k:v]",
	"X-H-Ma":           "true map[n:1 s:x]",
	"X-H-Fn":           "fn:x",
	"X-H-Err":          "err:bad",
	"X-H-User":         "user:bob",
	"X-H-Dur":          "dur",
	"X-H-Local":        "fromA",
	"X-B-Aval":         "trueavv",
	"X-B-Buf":          "truebufv",
	"X-B-Cookie":       "truecv",
	"X-B-D":            "true 3000000000",
	"X-B-Hdr":          "truehv",
	"X-B-Holder":       "true true true",
	"X-B-Holder-T":     "true true true",
	"X-B-Ip":           "true10.0.0.1",
	"X-B-Jwt":          "true true [g1 g2] true map[k:v]x",
	"X-B-Mx":           "true true [r1 r2] true map[k:v] true [map[a:1]] trueuser:nested",
	"X-B-Num":          "true12",
	"X-B-Pass":         "structintptrsptr",
	"X-B-Pass-Cancel":  "structintptrs",
	"X-B-Raw":          "true{\"a\":1}",
	"X-B-Req":          "trueGET",
	"X-B-Sb":           "truesbv",
	"X-B-Smap":         "truesmv",
	"X-B-Time":         "true 2024 5",
	"X-B-Url":          "true/p1",
	"X-B-Vals":         "true1",
	"X-H-Aval":         "trueavv",
	"X-H-Buf":          "truebufv",
	"X-H-Cookie":       "truecv",
	"X-H-D":            "true 3000000000",
	"X-H-Hdr":          "truehv",
	"X-H-Hdr-B":        "1",
	"X-H-Cookie-B":     "/b",
	"X-H-Url-B":        "b",
	"X-H-Ip":           "true10.0.0.1",
	"X-H-Jwt":          "true true [g1 g2] true map[k:v]x",
	"X-H-Mx":           "true true [r1 r2] true map[k:v] true [map[a:1]] trueuser:nested",
	"X-H-Num":          "true12",
	"X-H-Raw":          "true{\"a\":1}",
	"X-H-Req":          "trueGET",
	"X-H-Sb":           "truesbv",
	"X-H-Smap":         "truesmv",
	"X-H-Smap-B":       "1",
	"X-H-Time":         "true 2024 5",
	"X-H-Url":          "true/p1",
	"X-H-Vals":         "true1",
}

// ctxWantUnrestricted are the values of ctxWant that differ for
// unrestricted plugins.
var ctxWantUnrestricted = map[string]string{
	"X-A-Server": "*http.Server",
	"X-B-Server": "*http.Server",
}

// TestContextValuesAcrossMachines chains two plugins loaded with separate
// Loads (one machine each, as Traefik does) and a host handler, behind a
// real server. Before:
//   - Value(http.LocalAddrContextKey) and Value(http.ServerContextKey)
//     were nil in a plugin: the key, of the unexported native type
//     *net/http.contextKey, became a key that no native context matches;
//     the values that a plugin stored with a native key
//     (http.LocalAddrContextKey, time.Duration(3)) were invisible to
//     native code and to the next plugin;
//   - a string key of A was nil in B: ctxValue compared the key of A's
//     context with the types of B, then skipped it;
//   - a value of A that was not basic was a vm.vmOpaque for the host and
//     for B: their assertions to []string, map[string]any, func, error
//     and fmt.Stringer were false;
//   - B did not see the methods of A's type that the box of the value
//     did not have (Tag).
//
// Third-round review:
//   - with -aot, a slice or a map nested in a map[string]any of A (JWT
//     claims from json.Unmarshal) was a vm.vmOpaque for B and the host:
//     jwt["groups"].([]any) was false;
//   - a value of a native named type (time.Time, http.Header, *url.URL,
//     *sync.Map, json.Number...) was a wrapper or a handle for B and the
//     host: their assertions to its type were false;
//   - in B, a key of a named, pointer or struct type was nil through a
//     context wrapper of B under a native context (pass);
//   - a method of A with result any gave B a new native copy at each
//     call: two results that held the same pointer were not equal, and
//     the methods that the copy did not have were lost (GetT, Tag).
//
// The expected values are what Go gives for the same code (ctxCheck runs
// natively in the host, its text in B).
//
// A also reads back its own slice through a native context, and changes
// it: the next ones see the change, as in Go.
func TestContextValuesAcrossMachines(t *testing.T) {
	needSource(t)
	// B runs ctxCheckSrc, the host ctxCheck: the two must stay the same
	if src, err := os.ReadFile("ctxcheck_test.go"); err != nil || !strings.Contains(string(src), "\n"+ctxCheckSrc+"\n") {
		t.Fatalf("ctxCheckSrc is not the text of ctxCheck (%v)", err)
	}
	for _, tc := range []struct {
		name string
		opts Options
	}{
		{"vm", Options{}},
		{"aot", Options{AOT: true}},
		{"unrestricted", Options{Unrestricted: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := loadChainTo(t, tc.opts, http.HandlerFunc(ctxHostHandler), ctxSetSrc, ctxGetSrc)
			srv := httptest.NewServer(h)
			defer srv.Close()
			for range 3 {
				resp, err := http.Get(srv.URL)
				if err != nil {
					t.Fatal(err)
				}
				resp.Body.Close()
				for k, want := range ctxWant {
					if w, ok := ctxWantUnrestricted[k]; ok && tc.opts.Unrestricted {
						want = w
					}
					if got := resp.Header.Get(k); got != want {
						t.Errorf("%s = %q, want %q", k, got, want)
					}
				}
				if t.Failed() {
					return
				}
			}
		})
	}
}

// srvHijackSrc tries to take the entry point of the host: it gets the
// *http.Server from the context of the request and replaces its Handler.
const srvHijackSrc = `package p

import (
	"context"
	"net/http"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type handler struct{ next http.Handler }

func New(ctx context.Context, next http.Handler, cfg *Config, name string) (http.Handler, error) {
	return handler{next}, nil
}

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s, ok := r.Context().Value(http.ServerContextKey).(*http.Server); ok {
		s.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("HIJACKED"))
		})
	}
	h.next.ServeHTTP(w, r)
}
`

// TestRestrictedHostServerHijack runs a restricted plugin that sets the
// Handler of the *http.Server it reads with http.ServerContextKey, behind
// a real server. Before, keyNative gave the plugin the native key, so it
// got the server of the host: the second request went to plugin code
// ("HIJACKED") instead of the host handler.
func TestRestrictedHostServerHijack(t *testing.T) {
	needSource(t)
	for _, aot := range []bool{false, true} {
		t.Run(map[bool]string{false: "vm", true: "aot"}[aot], func(t *testing.T) {
			host := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("host")) })
			srv := httptest.NewServer(loadOne(t, Options{AOT: aot}, srvHijackSrc, host, "hijack"))
			defer srv.Close()
			for i := range 2 {
				resp, err := http.Get(srv.URL)
				if err != nil {
					t.Fatal(err)
				}
				b, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				if string(b) != "host" {
					t.Fatalf("request %d: body %q, want %q", i, b, "host")
				}
			}
		})
	}
}

// tagWrapSrc wraps the ResponseWriter with a type that has ReadFrom, as
// io.Copy uses it: ReadFrom asserts a method of the reader that io.Reader
// does not have, then copies to the writer it wraps.
const tagWrapSrc = `package p

import (
	"context"
	"io"
	"net/http"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type rw struct{ http.ResponseWriter }

func (w rw) ReadFrom(r io.Reader) (int64, error) {
	tag := "none"
	if t, ok := r.(interface{ Tag() string }); ok {
		tag = t.Tag()
	}
	w.ResponseWriter.Header().Add("X-Tag", tag)
	return io.Copy(w.ResponseWriter, r)
}

type handler struct{ next http.Handler }

func New(ctx context.Context, next http.Handler, cfg *Config, name string) (http.Handler, error) {
	return handler{next}, nil
}

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.next.ServeHTTP(rw{w}, r)
}
`

// tagLeafSrc copies a reader of its own type, with a Tag method, to the
// writer it gets.
const tagLeafSrc = `package p

import (
	"context"
	"io"
	"net/http"
	"strings"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type reader struct{ r io.Reader }

func (r *reader) Read(b []byte) (int, error) { return r.r.Read(b) }
func (r *reader) Tag() string                { return "leaf" }

type handler struct{}

func New(ctx context.Context, next http.Handler, cfg *Config, name string) (http.Handler, error) {
	return handler{}, nil
}

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	io.Copy(w, &reader{strings.NewReader("body")})
}
`

// TestForeignValueMethods chains wrap -> wrap -> leaf, one machine each.
// The reader of the leaf reaches the ReadFrom of each wrapper as an
// io.Reader wrapper, which only has Read: r.(interface{ Tag() string })
// was false (Go: true). The wrappers now see the methods of the leaf's
// type, and call Tag in the leaf's machine.
func TestForeignValueMethods(t *testing.T) {
	needSource(t)
	for _, aot := range []bool{false, true} {
		t.Run(map[bool]string{false: "vm", true: "aot"}[aot], func(t *testing.T) {
			h := loadChainTo(t, Options{AOT: aot}, http.NotFoundHandler(), tagWrapSrc, tagWrapSrc, tagLeafSrc)
			srv := httptest.NewServer(h)
			defer srv.Close()
			resp, err := http.Get(srv.URL)
			if err != nil {
				t.Fatal(err)
			}
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if got, want := strings.Join(resp.Header.Values("X-Tag"), ","), "leaf,leaf"; got != want {
				t.Errorf("X-Tag = %q, want %q", got, want)
			}
			if string(b) != "body" {
				t.Errorf("body %q, want %q", b, "body")
			}
		})
	}
}

// namedSetSrc stores a value whose methods use a named type of the plugin
// (T), and others whose methods do not.
const namedSetSrc = `package p

import (
	"context"
	"net/http"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type T struct{ N int }

type reader struct{}

func (r *reader) Get() T                 { return T{42} }
func (r *reader) Raw() struct{ N int }   { return struct{ N int }{7} }
func (r *reader) Num() int               { return 3 }

type handler struct{ next http.Handler }

func New(ctx context.Context, next http.Handler, cfg *Config, name string) (http.Handler, error) {
	return handler{next}, nil
}

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), "r", &reader{})))
}
`

// namedGetSrc declares its own T, identical to the T of namedSetSrc, and
// asserts the value of the other plugin to interfaces with it.
const namedGetSrc = `package p

import (
	"context"
	"fmt"
	"net/http"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type T struct{ N int }

type handler struct{ next http.Handler }

func New(ctx context.Context, next http.Handler, cfg *Config, name string) (http.Handler, error) {
	return handler{next}, nil
}

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	v := r.Context().Value("r")
	set := func(k string, v ...any) { w.Header().Set("X-"+k, fmt.Sprint(v...)) }
	if g, ok := v.(interface{ Get() T }); ok {
		set("Get", true, g.Get())
	} else {
		set("Get", false)
	}
	_, ok := v.(interface{ Get() struct{ N int } })
	set("Get-S", ok)
	_, ok = v.(interface{ Raw() T })
	set("Raw-T", ok)
	if g, ok := v.(interface{ Raw() struct{ N int } }); ok {
		set("Raw", true, g.Raw())
	} else {
		set("Raw", false)
	}
	if g, ok := v.(interface{ Num() int }); ok {
		set("Num", true, g.Num())
	} else {
		set("Num", false)
	}
	_, ok = v.(interface{ Num() string })
	set("Num-S", ok)
	_, ok = v.(interface{ Num(int) int })
	set("Num-Arg", ok)
	h.next.ServeHTTP(w, r)
}
`

// TestForeignNamedSignature passes a value of plugin A to plugin B, which
// declares its own T, identical to the T of A. In Go the two T are
// different types: B's assertion to interface{ Get() T } is false for
// A's Get() T, and so are interface{ Get() struct{ N int } } for Get() T
// and interface{ Raw() T } for Raw() struct{ N int }. A method of the
// same name with another signature without named types (Num() string or
// Num(int) int for Num() int) does not match either.
// Before, foreignHas only compared the native signatures, which are the
// same, so both were true and B called A's Get. Methods without named
// types of a plugin still cross.
func TestForeignNamedSignature(t *testing.T) {
	needSource(t)
	want := map[string]string{
		"X-Get":     "false",
		"X-Get-S":   "false",
		"X-Raw-T":   "false",
		"X-Raw":     "true {7}",
		"X-Num":     "true 3",
		"X-Num-S":   "false",
		"X-Num-Arg": "false",
	}
	for _, aot := range []bool{false, true} {
		t.Run(map[bool]string{false: "vm", true: "aot"}[aot], func(t *testing.T) {
			h := loadChainTo(t, Options{AOT: aot}, http.NotFoundHandler(), namedSetSrc, namedGetSrc)
			srv := httptest.NewServer(h)
			defer srv.Close()
			resp, err := http.Get(srv.URL)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			for k, w := range want {
				if got := resp.Header.Get(k); got != w {
					t.Errorf("%s = %q, want %q", k, got, w)
				}
			}
		})
	}
}

// ifaceResSetSrc stores a value whose method returns a value of the
// plugin as an io.Reader, a non-empty interface.
const ifaceResSetSrc = `package p

import (
	"context"
	"io"
	"net/http"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type rd struct{ done bool }

func (r *rd) Read(p []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}
	r.done = true
	return copy(p, "hello"), nil
}

type hold struct{}

func (hold) R() io.Reader { return &rd{} }

type handler struct{ next http.Handler }

func New(ctx context.Context, next http.Handler, cfg *Config, name string) (http.Handler, error) {
	return handler{next}, nil
}

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), "hold", hold{})))
}
`

// ifaceResGetSrc calls the method of the other plugin and reads the
// io.Reader it gets.
const ifaceResGetSrc = `package p

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type handler struct{ next http.Handler }

func New(ctx context.Context, next http.Handler, cfg *Config, name string) (http.Handler, error) {
	return handler{next}, nil
}

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if g, ok := r.Context().Value("hold").(interface{ R() io.Reader }); ok {
		b, err := io.ReadAll(g.R())
		w.Header().Set("X-B-Rd", fmt.Sprint(string(b), err))
	} else {
		w.Header().Set("X-B-Rd", "false")
	}
	h.next.ServeHTTP(w, r)
}
`

// TestForeignIfaceResult calls, in plugin B, a method of plugin A whose
// result is io.Reader. Before, the result was boxed as a value of A
// (vmOpaque) without looking at the result type: reflect.Set panicked in
// the host ("vm.vmOpaque is not assignable to type io.Reader") and the
// request died.
func TestForeignIfaceResult(t *testing.T) {
	needSource(t)
	for _, aot := range []bool{false, true} {
		t.Run(map[bool]string{false: "vm", true: "aot"}[aot], func(t *testing.T) {
			h := loadChainTo(t, Options{AOT: aot}, http.NotFoundHandler(), ifaceResSetSrc, ifaceResGetSrc)
			srv := httptest.NewServer(h)
			defer srv.Close()
			resp, err := http.Get(srv.URL)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if got, want := resp.Header.Get("X-B-Rd"), "hello<nil>"; got != want {
				t.Errorf("X-B-Rd = %q, want %q", got, want)
			}
		})
	}
}

// nestedTagSetSrc stores a value of the plugin with a method that no hand
// wrapper knows (Tag), nested in a map[string]any and in a []any.
const nestedTagSetSrc = `package p

import (
	"context"
	"net/http"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type tagged struct{ n int }

func (t *tagged) Tag() string { return "tag" }

type handler struct{ next http.Handler }

func New(ctx context.Context, next http.Handler, cfg *Config, name string) (http.Handler, error) {
	return handler{next}, nil
}

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := context.WithValue(r.Context(), "nt", map[string]any{"t": &tagged{5}})
	ctx = context.WithValue(ctx, "st", []any{&tagged{6}})
	h.next.ServeHTTP(w, r.WithContext(ctx))
}
`

// nestedTagGetSrc asserts the nested values of the other plugin to an
// interface with Tag.
const nestedTagGetSrc = `package p

import (
	"context"
	"fmt"
	"net/http"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type handler struct{ next http.Handler }

func New(ctx context.Context, next http.Handler, cfg *Config, name string) (http.Handler, error) {
	return handler{next}, nil
}

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	m, _ := ctx.Value("nt").(map[string]any)
	t, ok := m["t"].(interface{ Tag() string })
	w.Header().Set("X-B-Nt", fmt.Sprint(ok, ok && t.Tag() == "tag"))
	s, _ := ctx.Value("st").([]any)
	ok = false
	if len(s) == 1 {
		t, ok = s[0].(interface{ Tag() string })
	}
	w.Header().Set("X-B-St", fmt.Sprint(ok, ok && t.Tag() == "tag"))
	h.next.ServeHTTP(w, r)
}
`

// TestContextNestedMethods passes a value of plugin A with a method that
// no hand wrapper knows, nested in a map[string]any and a []any, to
// plugin B. In Go, B's assertion to interface{ Tag() string } is true.
// Before, the nested values became native copies without methods (in
// AOT mode since the third-round fix of nested slices and maps, in VM
// mode always): the assertion was false.
func TestContextNestedMethods(t *testing.T) {
	needSource(t)
	for _, aot := range []bool{false, true} {
		t.Run(map[bool]string{false: "vm", true: "aot"}[aot], func(t *testing.T) {
			h := loadChainTo(t, Options{AOT: aot}, http.NotFoundHandler(), nestedTagSetSrc, nestedTagGetSrc)
			srv := httptest.NewServer(h)
			defer srv.Close()
			resp, err := http.Get(srv.URL)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			for _, k := range []string{"X-B-Nt", "X-B-St"} {
				if got, want := resp.Header.Get(k), "true true"; got != want {
					t.Errorf("%s = %q, want %q", k, got, want)
				}
			}
		})
	}
}

// loadChainTo is loadChain with the handler after the last plugin.
func loadChainTo(t *testing.T, opts Options, last http.Handler, srcs ...string) http.Handler {
	t.Helper()
	next := last
	for i := len(srcs) - 1; i >= 0; i-- {
		next = loadOne(t, opts, srcs[i], next, fmt.Sprint("chain", i))
	}
	return next
}
