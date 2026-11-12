// Native boundary of net/rpc: program types given to net/rpc (docs/BOUNDARY.md).
package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"sort"
	"strings"
	"sync"
)

var cur string

// probe runs one probe: its lines start with its id, and its panic is printed.
func probe(id string, f func()) {
	cur = id
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("%s: panic: %T: %v\n", id, r, r)
		}
	}()
	f()
}

// out prints one line of the current probe.
func out(format string, args ...any) { fmt.Printf(cur+": "+format+"\n", args...) }

// Args are the arguments of the Arith methods.
type Args struct{ A, B int }

// Quo is a reply struct.
type Quo struct{ Q, R int }

// Num is a named int argument.
type Num int

// DivErr is a program error type.
type DivErr struct{ Msg string }

func (e *DivErr) Error() string { return "DivErr: " + e.Msg }

// Arith is a service with pointer receivers.
type Arith struct{ calls int }

func (t *Arith) Mul(args Args, reply *int) error {
	*reply = args.A * args.B
	return nil
}

func (t *Arith) Div(args *Args, quo *Quo) error {
	if args.B == 0 {
		return &DivErr{"divide by zero"}
	}
	quo.Q, quo.R = args.A/args.B, args.A%args.B
	return nil
}

func (t *Arith) Double(n Num, reply *Num) error { *reply = 2 * n; return nil }

func (t *Arith) Split(s string, reply *[]string) error {
	*reply = strings.Split(s, ",")
	return nil
}

func (t *Arith) Count(s string, reply *map[string]int) error {
	for _, w := range strings.Fields(s) {
		(*reply)[w]++
	}
	return nil
}

func (t *Arith) Fail(n int, reply *int) error { return errors.New("plain failure") }

// unexported methods and bad signatures are not service methods
func (t *Arith) helper(args Args, reply *int) error { return nil }
func (t *Arith) Bad1(args Args) error               { return nil }
func (t *Arith) Bad2(args Args, reply int) error    { return nil }
func (t *Arith) Bad3(args Args, reply *int) int     { return 0 }

// Echo is a service with value receivers.
type Echo struct{ prefix string }

func (e Echo) Say(s string, reply *string) error { *reply = e.prefix + s; return nil }

// Gen is a generic service.
type Gen[T any] struct{ v T }

func (g *Gen[T]) Get(n int, reply *string) error { *reply = fmt.Sprintf("%v/%d", g.v, n); return nil }

// NoMethods has no service method.
type NoMethods struct{}

func (NoMethods) Bad(a int) error { return nil }

// hidden is not exported.
type hidden struct{}

func (*hidden) Get(n int, reply *int) error { return nil }

// codec is an in-memory rpc.ServerCodec: it serves a list of requests.
type codec struct {
	mu   sync.Mutex
	reqs []req
	resp []string
	cur  req
}

type req struct {
	method string
	body   any
}

func (c *codec) ReadRequestHeader(r *rpc.Request) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.reqs) == 0 {
		return io.EOF
	}
	c.cur, c.reqs = c.reqs[0], c.reqs[1:]
	r.ServiceMethod = c.cur.method
	r.Seq = uint64(len(c.resp) + len(c.reqs))
	return nil
}

func (c *codec) ReadRequestBody(body any) error {
	if body == nil {
		return nil
	}
	switch b := body.(type) {
	case *Args:
		*b = c.cur.body.(Args)
	case *string:
		*b = c.cur.body.(string)
	case *Num:
		*b = c.cur.body.(Num)
	case *int:
		*b = c.cur.body.(int)
	default:
		return fmt.Errorf("body type %T", body)
	}
	return nil
}

func (c *codec) WriteResponse(r *rpc.Response, body any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	v := body
	switch b := body.(type) {
	case *int:
		v = *b
	case *Quo:
		v = *b
	case *string:
		v = *b
	case *Num:
		v = *b
	case *[]string:
		v = *b
	case *map[string]int:
		v = *b
	}
	c.resp = append(c.resp, fmt.Sprintf("%s err=%q body=%T %v", r.ServiceMethod, r.Error, body, v))
	return nil
}

func (c *codec) Close() error { return nil }

func (c *codec) results() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	r := append([]string(nil), c.resp...)
	sort.Strings(r)
	return r
}

func requests() []req {
	return []req{
		{"Arith.Mul", Args{6, 7}},
		{"Arith.Div", Args{17, 5}},
		{"Arith.Div", Args{1, 0}},
		{"Arith.Double", Num(21)},
		{"Arith.Split", "a,b,c"},
		{"Arith.Count", "x y x"},
		{"Arith.Fail", 1},
		{"Arith.Nope", 1},
		{"Echo.Say", "hi"},
	}
}

// logs captures the output of the log package.
var logs strings.Builder

// calls makes the client calls and prints the results.
func calls(c *rpc.Client, name string) {
	var p int
	err := c.Call(name+".Mul", Args{6, 7}, &p)
	out("Mul: %v %v", p, err)
	var q Quo
	err = c.Call(name+".Div", &Args{17, 5}, &q)
	out("Div: %v %v", q, err)
	err = c.Call(name+".Div", Args{1, 0}, &q)
	out("Div 0: %v %T", err, err)
	var n Num
	err = c.Call(name+".Double", Num(4), &n)
	out("Double: %v %v", n, err)
	var parts []string
	err = c.Call(name+".Split", "a,b", &parts)
	out("Split: %v %v", parts, err)
	var m map[string]int
	err = c.Call(name+".Count", "a b a", &m)
	out("Count: %v %v", m, err)
	err = c.Call(name+".Fail", 1, &p)
	out("Fail: %v", err)
	err = c.Call(name+".Bad2", Args{}, &p)
	out("Bad2: %v", err)
}

// listener is a program net.Listener that gives one connection.
type listener struct {
	conns chan net.Conn
	done  chan struct{}
	once  sync.Once
}

func (l *listener) Accept() (net.Conn, error) {
	select {
	case c := <-l.conns:
		return c, nil
	case <-l.done:
		return nil, errors.New("listener closed")
	}
}
func (l *listener) Close() error   { l.once.Do(func() { close(l.done) }); return nil }
func (l *listener) Addr() net.Addr { return addr{} }

type addr struct{}

func (addr) Network() string { return "mem" }
func (addr) String() string  { return "mem" }

// rw is a program http.ResponseWriter without Hijack.
type rw struct {
	h    http.Header
	code int
	body strings.Builder
}

func (w *rw) Header() http.Header         { return w.h }
func (w *rw) Write(p []byte) (int, error) { return w.body.Write(p) }
func (w *rw) WriteHeader(code int)        { w.code = code }

// hrw is a program http.ResponseWriter with Hijack.
type hrw struct {
	rw
	conn net.Conn
	err  error
}

func (w *hrw) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if w.err != nil {
		return nil, nil, w.err
	}
	return w.conn, bufio.NewReadWriter(bufio.NewReader(w.conn), bufio.NewWriter(w.conn)), nil
}

func init() {
	log.SetFlags(0)
	log.SetOutput(&logs)
}

// printLogs prints and clears the captured log lines.
func printLogs() {
	for _, l := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		if l != "" {
			out("log: %s", l)
		}
	}
	logs.Reset()
}

func main() {
	// row: net/rpc (*Server).Register,(*Server).RegisterName reflect *
	probe("Server.Register", func() {
		s := rpc.NewServer()
		out("Arith: %v", s.Register(&Arith{}))
		out("Arith again: %v", s.Register(&Arith{}))
		out("Echo value: %v", s.Register(Echo{"e:"}))
		out("Gen: %v", s.RegisterName("Gen", &Gen[string]{"g"}))
		out("Gen no name: %v", s.Register(&Gen[int]{1}))
		out("NoMethods: %v", s.Register(NoMethods{}))
		out("Arith value: %v", s.RegisterName("ArithV", Arith{}))
		out("hidden: %v", s.Register(&hidden{}))
		out("hidden named: %v", s.RegisterName("Hidden", &hidden{}))
		out("empty name: %v", s.RegisterName("", &Arith{}))
		printLogs()
	})
	// row: net/rpc Register,RegisterName reflect *
	probe("Register", func() {
		out("Arith: %v", rpc.Register(&Arith{}))
		out("Echo: %v", rpc.RegisterName("Echo", Echo{"d:"}))
		out("NoMethods: %v", rpc.Register(&NoMethods{}))
		out("Gen: %v", rpc.RegisterName("G", &Gen[bool]{true}))
		printLogs()
	})
	// row: net/rpc (*Server).ServeRequest,ServeRequest assert x.(error)
	// row: net/rpc (*Server).ServeRequest,ServeRequest panic x.(error) without ok: *runtime.TypeAssertionError
	// row: net/rpc (*Server).ServeRequest,ServeRequest reflect *
	probe("ServeRequest", func() {
		s := rpc.NewServer()
		s.Register(&Arith{})
		s.Register(Echo{"e:"})
		c := &codec{reqs: requests()}
		for i := 0; i < len(requests())+1; i++ {
			out("%d: %v", i, s.ServeRequest(c))
		}
		for _, r := range c.results() {
			out("%s", r)
		}
		c = &codec{reqs: requests()[:2]}
		out("default: %v %v", rpc.ServeRequest(c), c.results())
		printLogs()
	})
	// row: net/rpc (*Server).ServeCodec,ServeCodec assert x.(error)
	// row: net/rpc (*Server).ServeCodec,ServeCodec panic x.(error) without ok: *runtime.TypeAssertionError
	// row: net/rpc (*Server).ServeCodec,ServeCodec reflect *
	probe("ServeCodec", func() {
		s := rpc.NewServer()
		s.Register(&Arith{})
		s.Register(Echo{"e:"})
		c := &codec{reqs: requests()}
		s.ServeCodec(c)
		for _, r := range c.results() {
			out("%s", r)
		}
		c = &codec{reqs: requests()[3:5]}
		rpc.ServeCodec(c)
		out("default: %v", c.results())
		printLogs()
	})
	// row: net/rpc (*Server).ServeConn,ServeConn assert x.(error)
	// row: net/rpc (*Server).ServeConn,ServeConn panic x.(error) without ok: *runtime.TypeAssertionError
	// row: net/rpc (*Server).ServeConn,ServeConn reflect *
	probe("ServeConn", func() {
		s := rpc.NewServer()
		s.Register(&Arith{})
		cs, cc := net.Pipe()
		done := make(chan struct{})
		go func() { s.ServeConn(cs); close(done) }()
		c := rpc.NewClient(cc)
		calls(c, "Arith")
		var r string
		out("Echo missing: %v", c.Call("Echo.Say", "x", &r))
		c.Close()
		<-done
		cs, cc = net.Pipe()
		done = make(chan struct{})
		go func() { rpc.ServeConn(cs); close(done) }()
		c = rpc.NewClient(cc)
		out("default Echo: %v", c.Call("Echo.Say", "x", &r))
		out("default Echo reply: %q", r)
		calls(c, "Arith")
		c.Close()
		<-done
		printLogs()
	})
	// row: net/rpc (*Server).Accept,Accept assert x.(error)
	// row: net/rpc (*Server).Accept,Accept panic x.(error) without ok: *runtime.TypeAssertionError
	// row: net/rpc (*Server).Accept,Accept reflect *
	probe("Accept", func() {
		s := rpc.NewServer()
		s.RegisterName("A", &Arith{})
		for _, name := range []string{"server", "default"} {
			l := &listener{conns: make(chan net.Conn, 1), done: make(chan struct{})}
			cs, cc := net.Pipe()
			l.conns <- cs
			done := make(chan struct{})
			go func() {
				if name == "server" {
					s.Accept(l)
				} else {
					rpc.Accept(l)
				}
				close(done)
			}()
			c := rpc.NewClient(cc)
			if name == "server" {
				calls(c, "A")
			} else {
				calls(c, "Arith")
			}
			c.Close()
			l.Close()
			<-done
		}
		printLogs()
	})
	// row: net/rpc (*Server).ServeHTTP assert *
	// row: net/rpc (*Server).ServeHTTP panic *
	// row: net/rpc (*Server).ServeHTTP reflect *
	probe("ServeHTTP", func() {
		s := rpc.NewServer()
		s.Register(&Arith{})
		get, _ := http.NewRequest("GET", "/_goRPC_", nil)
		w := &rw{h: http.Header{}}
		s.ServeHTTP(w, get)
		out("GET: %d %q %q", w.code, w.h.Get("Content-Type"), w.body.String())
		conn, _ := http.NewRequest("CONNECT", "/_goRPC_", nil)
		hw := &hrw{rw: rw{h: http.Header{}}, err: errors.New("no hijack")}
		s.ServeHTTP(hw, conn)
		printLogs()
		cs, cc := net.Pipe()
		hw = &hrw{rw: rw{h: http.Header{}}, conn: cs}
		done := make(chan struct{})
		go func() { s.ServeHTTP(hw, conn); close(done) }()
		status := make([]byte, len("HTTP/1.0 200 Connected to Go RPC\n\n"))
		_, err := io.ReadFull(cc, status)
		out("status: %q %v", status, err)
		c := rpc.NewClient(cc)
		calls(c, "Arith")
		c.Close()
		<-done
		func() {
			defer func() {
				r := recover()
				out("no Hijacker: panic %T: %v", r, r)
			}()
			s.ServeHTTP(&rw{h: http.Header{}}, conn)
		}()
	})
}
