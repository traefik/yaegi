package vm

// Process-wide registries of the standard library that a plugin writes to.
//
// In Go, http.Handle, expvar.Publish, sql.Register, mime.AddExtensionType,
// image.RegisterFormat and rpc.Register add an entry to a table of the
// process, and http.DefaultClient and http.DefaultTransport are objects of
// the process. A plugin that runs natively in the host shares them with the
// host and with the other plugins: its routes were served by the host when
// the host serves http.DefaultServeMux, two plugins that register the same
// pattern, expvar name or sql driver made the second load panic, a mime
// type of a plugin changed the content types of the host's file servers, an
// image format of a plugin made the host's image.Decode run plugin code, and
// http.DefaultTransport.(*http.Transport).TLSClientConfig = ... in a plugin
// changed the TLS checks of the whole host.
//
// A restricted machine (not m.realStd, like flag.CommandLine in exit.go)
// has its own copy of each of them, made at the first use:
//
//   - net/http: its own DefaultServeMux, DefaultClient and DefaultTransport
//     variables. Handle and HandleFunc register on the machine's mux, and a
//     nil handler means that mux in ListenAndServe, ListenAndServeTLS,
//     Serve, ServeTLS, the Serve, ServeTLS, ListenAndServe and
//     ListenAndServeTLS methods of *http.Server (they set Server.Handler),
//     the Start and StartTLS methods of *httptest.Server (they set
//     Config.Handler), httptest.NewServer, NewTLSServer,
//     NewUnstartedServer, fcgi.Serve and cgi.Serve. A nil Handler that the
//     plugin stores into a server it started also means that mux
//     (lateNilHandler), and the plugin reads nil there, as in Go. When the
//     program imports expvar or net/http/pprof, the
//     machine's mux has their handlers, as in Go. DefaultTransport starts as
//     a clone of the host's *http.Transport. DefaultClient is a new
//     &http.Client{}, with a nil Transport as in Go. The Do, Get, Head,
//     Post, PostForm and CloseIdleConnections methods of a client with a
//     nil Transport use the machine's DefaultTransport (in Go, a nil
//     Transport means the host's), and http.Get, Head, Post and PostForm
//     use the machine's DefaultClient. The ServeHTTP method of an
//     httputil.ReverseProxy with a nil Transport also uses the machine's
//     DefaultTransport.
//   - expvar: its own table of variables; Handler and the /debug/vars route
//     of its mux serve it. The table of an Unrestricted machine starts with
//     the host's cmdline and memstats, as in Go; the table of a machine with
//     the load-time restrictions starts empty: the plugin sees nothing of
//     the host. The table of expvar is in stdsyms/sideeffect, which the
//     host must import (checkSideEffectStd). Restricted plugins may not
//     import net/http/pprof (refusedImports in restrict.go).
//   - database/sql: Register stores the driver in the machine's table, Open
//     opens it as sql.Open does (sql.OpenDB), and Drivers shows the
//     machine's names and the host's. A name that the machine did not
//     register opens a driver of the host, as before.
//   - mime: AddExtensionType adds to the machine's table; TypeByExtension
//     and ExtensionsByType read it first, then the host's.
//   - image: RegisterFormat adds to the machine's list; Decode and
//     DecodeConfig try the host's formats, then the machine's.
//   - net/rpc: its own DefaultServer; the package functions use it, and
//     HandleHTTP registers on the machine's mux.
//   - crypto: RegisterHash adds to the machine's table; the New and
//     Available methods of crypto.Hash read it first. A plugin cannot
//     replace the SHA-256 of the host's TLS and x509 checks.
//   - crypto/x509: SetFallbackRoots keeps the checks of Go (nil roots,
//     a second call) but does not change the host's roots.
//   - archive/zip: RegisterCompressor and RegisterDecompressor add to the
//     machine's tables; NewReader, OpenReader and NewWriter give them to
//     the reader or writer they return.
//
// A machine with the load-time restrictions (m.restricted: Load without
// Unrestricted, RunMain with Restricted) also gets:
//
//   - net: its own DefaultResolver, a copy of the host's at the first use
//     (net.DefaultResolver.Dial = ... would redirect the DNS of the host).
//   - runtime: GOMAXPROCS(n) with n > 0 does not change the setting of the
//     process: it returns the current value, as GOMAXPROCS(0) does.
//     Libraries call GOMAXPROCS(0) to size their pools, and
//     go.uber.org/automaxprocs sets it at init: a panic would stop such a
//     plugin from loading, so the call does nothing instead.
//
// The other writes to the standard library variables and to the objects
// they reach are refused: at load time when the code names the variable
// (checkWrites in restrict.go), at run time otherwise (hostmem.go).
//
// The AOT copies of database/sql and net/rpc get the same through their
// hooks (hostHook), and aotgen rewrites the calls of http.Handle,
// http.HandleFunc and the other nil-handler functions, and the reads of
// http.DefaultServeMux, of the copies into AOTHandle... below.
//
// Limits: native code that the plugin calls still uses the host's tables:
// http.FileServer and http.ServeContent read the host's mime types, native
// crypto code (rsa.SignPSS, x509, tls) the host's hashes and roots, and
// native code that calls the methods of a *http.Client with a nil
// Transport (an AOT copy, a stdlib function), or the ServeHTTP of an
// httputil.ReverseProxy with a nil Transport that native code calls (the
// proxy given as an http.Handler), uses the host's DefaultTransport (not
// modified by the plugin). In native
// mode, the debug page of (*rpc.Server).HandleHTTP is not available (the
// handler type of net/rpc is not exported): its route answers 501. The
// AOT copy of net/rpc has the full page. TestPluginHostRegistries checks
// all of this.

import (
	"archive/zip"
	"bufio"
	"context"
	"crypto"
	"crypto/x509"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"hash"
	"image"
	"io"
	"maps"
	"mime"
	"net"
	"net/http"
	"net/http/cgi"
	"net/http/fcgi"
	"net/http/httptest"
	"net/http/httputil"
	"net/rpc"
	"net/rpc/jsonrpc"
	"net/url"
	"path"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"
	"weak"

	"github.com/traefik/yaegi/v2/stdsyms"
)

// hostRegs holds the registries of one restricted machine.
type hostRegs struct {
	mu      sync.Mutex
	funcs   map[string]reflect.Value    // hostFunc
	hooks   map[string]NativeHook       // hostHook
	methods map[methodKey]reflect.Value // hostMethod

	httpOnce sync.Once
	http     *httpDefaults

	expvarOnce sync.Once
	expvar     *expvarState

	sqlOnce sync.Once
	sql     *sqlState

	rpcOnce   sync.Once
	rpcServer reflect.Value // the address of the machine's rpc.DefaultServer

	resolverOnce sync.Once
	resolver     *net.Resolver // net.DefaultResolver

	mime  mimeState
	image imageState

	mu2           sync.Mutex // the fields below
	hashes        map[crypto.Hash]func() hash.Hash
	fallbackRoots bool
	compressors   map[uint16]zip.Compressor
	decompressors map[uint16]zip.Decompressor
}

// hostRegs returns the registries of m.
func (m *Machine) hostRegs() *hostRegs {
	m.regsOnce.Do(func() {
		m.regs = &hostRegs{
			funcs:   map[string]reflect.Value{},
			hooks:   map[string]NativeHook{},
			methods: map[methodKey]reflect.Value{},
			mime:    mimeState{types: map[string]string{}, lower: map[string]string{}, exts: map[string][]string{}},
		}
	})
	return m.regs
}

// native reports whether the package path runs natively in m.
func (m *Machine) native(path string) bool { return m.modeOf(path) == ModeNative }

// hostFunc returns the machine's own version of the native function key
// ("net/http.Handle"), when the function writes to a registry of the
// process.
func (m *Machine) hostFunc(key string) (reflect.Value, bool) {
	i := strings.LastIndexByte(key, '.')
	if m.realStd || i < 0 || !hostPkgs[key[:i]] {
		return reflect.Value{}, false
	}
	r := m.hostRegs()
	r.mu.Lock()
	fn, ok := r.funcs[key]
	r.mu.Unlock()
	if !ok {
		fn = m.makeHostFunc(key[:i], key[i+1:])
		r.mu.Lock()
		r.funcs[key] = fn
		r.mu.Unlock()
	}
	return fn, fn.IsValid()
}

// hostPkgs are the packages of makeHostFunc.
var hostPkgs = map[string]bool{
	"net/http": true, "net/http/httptest": true, "net/http/fcgi": true, "net/http/cgi": true,
	"expvar": true, "mime": true, "image": true, "database/sql": true, "net/rpc": true,
	"net/rpc/jsonrpc": true, "crypto": true, "crypto/x509": true, "archive/zip": true,
	"runtime": true,
}

// makeHostFunc returns the machine's version of pkg.name, or an invalid
// value when the function has none.
func (m *Machine) makeHostFunc(pkg, name string) reflect.Value {
	var fn any
	switch pkg {
	case "net/http":
		if m.native("net/http") {
			fn = m.httpFunc(name)
		}
	case "net/http/httptest", "net/http/fcgi", "net/http/cgi":
		if m.native("net/http") && m.native(pkg) {
			fn = m.httpServeFunc(pkg, name)
		}
	case "expvar":
		if m.native("expvar") && m.native("net/http") && expvarSyms() != nil {
			if f := m.expvarFunc(name); f.IsValid() {
				fn = f.Interface()
			}
		} else if m.restricted && m.native("expvar") && m.expvarFunc(name).IsValid() {
			// Native expvar over an interpreted net/http (Options.Modes, Src): the
			// machine's table needs the native mux, and the native function
			// is the host's table (its cmdline and memstats, names shared by
			// all plugins). A restricted plugin may not use it.
			unsupported("expvar.%s in a restricted plugin with net/http not native (Options.Modes, Options.Src): it would use the host's expvar table", name)
		}
	case "mime":
		if m.native("mime") {
			fn = m.mimeFunc(name)
		}
	case "image":
		if m.native("image") {
			fn = m.imageFunc(name)
		}
	case "database/sql":
		if m.native("database/sql") {
			fn = m.sqlFunc(name)
		}
	case "net/rpc":
		if m.native("net/rpc") && m.native("net/http") {
			fn = m.rpcFunc(name)
		}
	case "crypto", "crypto/x509", "archive/zip":
		if m.native(pkg) {
			fn = m.cryptoZipFunc(pkg, name)
		}
	case "net/rpc/jsonrpc":
		if name == "ServeConn" && m.native("net/rpc") && m.native("net/rpc/jsonrpc") {
			fn = func(conn io.ReadWriteCloser) { m.rpcDefault().ServeCodec(jsonrpc.NewServerCodec(conn)) }
		}
	case "runtime":
		if name == "GOMAXPROCS" && m.restricted && m.native("runtime") {
			fn = restrictedGOMAXPROCS
		}
	}
	if fn == nil || reflect.ValueOf(fn).IsNil() {
		return reflect.Value{}
	}
	return reflect.ValueOf(fn)
}

// hostHook returns the machine's own hook of the AOT function name, when
// the function writes to a registry of the process (database/sql and
// net/rpc).
func (m *Machine) hostHook(name string) (NativeHook, bool) {
	if m.realStd || !strings.HasPrefix(name, "database/sql.") && !strings.HasPrefix(name, "net/rpc.") {
		return nil, false
	}
	r := m.hostRegs()
	r.mu.Lock()
	h, ok := r.hooks[name]
	r.mu.Unlock()
	if !ok {
		i := strings.LastIndexByte(name, '.')
		if name[:i] == "database/sql" {
			h = m.sqlHook(name[i+1:])
		} else {
			h = m.rpcHook(name[i+1:])
		}
		r.mu.Lock()
		r.hooks[name] = h
		r.mu.Unlock()
	}
	return h, h != nil
}

// origHook returns the hook that the AOT copy registered for name.
func origHook(name string) NativeHook {
	regMu.RLock()
	defer regMu.RUnlock()
	h := nativeHooks[name]
	if h == nil {
		unsupported("AOT hook %s not registered", name)
	}
	return h
}

// hostMethod returns the machine's own version of the method name of the
// native type t, when it uses a registry of the process: the methods of
// *http.Server and *httptest.Server that serve http.DefaultServeMux for a
// nil Handler, the methods of *http.Client that use http.DefaultTransport
// for a nil Transport, (*httputil.ReverseProxy).ServeHTTP,
// (*rpc.Server).HandleHTTP, and
// crypto.Hash.New/Available.
func (m *Machine) hostMethod(t reflect.Type, name string) (reflect.Value, bool) {
	if m.realStd {
		return reflect.Value{}, false
	}
	var fn any
	switch {
	case t == reflect.TypeFor[*http.Server]() && m.native("net/http"):
		switch name {
		case "Serve":
			fn = func(s *http.Server, l net.Listener) error { m.fillHandler(s); return s.Serve(l) }
		case "ServeTLS":
			fn = func(s *http.Server, l net.Listener, cert, key string) error {
				m.fillHandler(s)
				return s.ServeTLS(l, cert, key)
			}
		case "ListenAndServe":
			fn = func(s *http.Server) error { m.fillHandler(s); return s.ListenAndServe() }
		case "ListenAndServeTLS":
			fn = func(s *http.Server, cert, key string) error {
				m.fillHandler(s)
				return s.ListenAndServeTLS(cert, key)
			}
		}
	case t == reflect.TypeFor[*http.Client]() && m.native("net/http"):
		fn = m.httpDefaults().clientMethod(name)
	case t == reflect.TypeFor[*httputil.ReverseProxy]() && name == "ServeHTTP" && m.native("net/http") && m.native("net/http/httputil"):
		fn = m.httpDefaults().proxyServeHTTP
	case t == reflect.TypeFor[*httptest.Server]() && m.native("net/http") && m.native("net/http/httptest"):
		// Start and StartTLS call the native s.Config.Serve, which does
		// not go through the case above. A nil s.Config panics in Go too.
		switch name {
		case "Start":
			fn = func(s *httptest.Server) {
				if s.Config != nil {
					m.fillHandler(s.Config)
				}
				s.Start()
			}
		case "StartTLS":
			fn = func(s *httptest.Server) {
				if s.Config != nil {
					m.fillHandler(s.Config)
				}
				s.StartTLS()
			}
		}
	case t == reflect.TypeFor[*rpc.Server]() && name == "HandleHTTP" && m.native("net/rpc") && m.native("net/http"):
		fn = m.rpcHandleHTTP
	case t == reflect.TypeFor[crypto.Hash]() && m.native("crypto"):
		r := m.hostRegs()
		switch name {
		case "New":
			fn = func(h crypto.Hash) hash.Hash {
				if f := r.hash(h); f != nil {
					return f()
				}
				return h.New()
			}
		case "Available":
			fn = func(h crypto.Hash) bool { return r.hash(h) != nil || h.Available() }
		}
	}
	if fn == nil {
		return reflect.Value{}, false
	}
	r := m.hostRegs()
	k := methodKey{t, name}
	r.mu.Lock()
	defer r.mu.Unlock()
	if rv, ok := r.methods[k]; ok {
		return rv, true
	}
	rv := reflect.ValueOf(fn)
	r.methods[k] = rv
	return rv, true
}

// hostVar returns the address of the machine's own variable pkg.name, when
// it is a registry of the process.
func (m *Machine) hostVar(pkg, name string) (reflect.Value, bool) {
	switch {
	case pkg == "net/http" && m.native("net/http"):
		st := m.httpDefaults()
		switch name {
		case "DefaultServeMux":
			return reflect.ValueOf(&st.mux), true
		case "DefaultClient":
			return reflect.ValueOf(&st.client), true
		case "DefaultTransport":
			return reflect.ValueOf(&st.transport), true
		}
	case pkg == "net/rpc" && name == "DefaultServer" && m.modeOf("net/rpc") != ModeInterp && m.native("net/http"):
		return m.rpcServerVar(), true
	case pkg == "net" && name == "DefaultResolver" && m.restricted && m.native("net"):
		r := m.hostRegs()
		r.resolverOnce.Do(func() {
			h := net.DefaultResolver
			r.resolver = &net.Resolver{PreferGo: h.PreferGo, StrictErrors: h.StrictErrors, Dial: h.Dial}
		})
		return reflect.ValueOf(&r.resolver), true
	}
	return reflect.Value{}, false
}

// restrictedGOMAXPROCS is runtime.GOMAXPROCS for a restricted machine: it
// never changes the setting.
func restrictedGOMAXPROCS(int) int { return runtime.GOMAXPROCS(0) }

// ---- net/http ----

// httpDefaults holds the variables DefaultServeMux, DefaultClient and
// DefaultTransport of package net/http for one machine.
type httpDefaults struct {
	mux       *http.ServeMux
	client    *http.Client
	transport http.RoundTripper
	handler   http.Handler // the nil handler: the current mux
}

// defaultMux serves the current value of the machine's DefaultServeMux,
// as a nil handler serves http.DefaultServeMux in Go.
type defaultMux struct{ st *httpDefaults }

var defaultMuxType = reflect.TypeFor[defaultMux]()

func (d defaultMux) ServeHTTP(w http.ResponseWriter, r *http.Request) { d.st.mux.ServeHTTP(w, r) }

// defaultTransport sends the requests of the machine's DefaultClient
// through the current value of its DefaultTransport, as a nil Transport
// does in Go.
type defaultTransport struct{ st *httpDefaults }

func (d defaultTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	t := d.st.transport
	if t == nil {
		// As (*http.Client).Do when Transport and DefaultTransport are nil.
		if r.Body != nil {
			r.Body.Close()
		}
		return nil, errors.New("http: no Client.Transport or DefaultTransport")
	}
	return t.RoundTrip(r)
}

func (d defaultTransport) CloseIdleConnections() {
	if c, ok := d.st.transport.(interface{ CloseIdleConnections() }); ok {
		c.CloseIdleConnections()
	}
}

// httpDefaults returns the net/http variables of m.
func (m *Machine) httpDefaults() *httpDefaults {
	r := m.hostRegs()
	r.httpOnce.Do(func() {
		st := &httpDefaults{mux: http.NewServeMux(), transport: http.DefaultTransport}
		if t, ok := http.DefaultTransport.(*http.Transport); ok {
			c := t.Clone()
			st.transport = c
			// Each idle keep-alive connection of the clone keeps two
			// goroutines until IdleConnTimeout. Close them when the
			// machine is gone (a plugin dropped at a reload), instead of
			// leaving them to the timeout.
			runtime.AddCleanup(m, func(c *http.Transport) { c.CloseIdleConnections() }, c)
		}
		// A nil Transport, as in Go: fmt prints the same value, and the
		// methods of clientMethod send through the machine's transport.
		st.client = &http.Client{}
		st.handler = defaultMux{st}
		// As the init functions of expvar and net/http/pprof do on the
		// DefaultServeMux of the host. Package vm does not import them
		// (TestHostImport): their tables come from stdsyms/sideeffect,
		// which the host imports when its programs may use them.
		if m.prog.ImportedPackage("expvar") != nil && m.native("expvar") && expvarSyms() != nil {
			st.mux.Handle(muxGET()+"/debug/vars", m.expvarHandler())
		}
		if pp := stdsyms.Symbols["net/http/pprof/pprof"]; m.prog.ImportedPackage("net/http/pprof") != nil && m.native("net/http/pprof") && pp != nil {
			p := muxGET()
			for _, route := range [][2]string{{"", "Index"}, {"cmdline", "Cmdline"}, {"profile", "Profile"}, {"symbol", "Symbol"}, {"trace", "Trace"}} {
				if h, ok := pp[route[1]].Interface().(func(http.ResponseWriter, *http.Request)); ok {
					st.mux.HandleFunc(p+"/debug/pprof/"+route[0], h)
				}
			}
		}
		r.http = st
	})
	return r.http
}

// via returns c, or a copy of c that sends its requests through the
// machine's DefaultTransport when c.Transport is nil: in Go, a nil
// Transport means the current http.DefaultTransport, and for a plugin that
// is its own copy. The copy has the other fields of c (Timeout, Jar,
// CheckRedirect) at the time of the call.
//
// The copy gets the current value of the machine's DefaultTransport itself,
// not the defaultTransport wrapper: net/http knows an *http.Transport, and
// for any other RoundTripper it cancels a Client.Timeout the legacy way
// (Request.Cancel, and one more goroutine and timer per request).
func (st *httpDefaults) via(c *http.Client) *http.Client {
	if c == nil || c.Transport != nil {
		return c // a nil c panics as in Go
	}
	cc := *c
	cc.Transport = defaultTransport{st}
	if st.transport != nil {
		cc.Transport = st.transport
	}
	return &cc
}

// clientMethod returns the machine's version of the method name of
// *http.Client, or nil when the method does not use the transport.
func (st *httpDefaults) clientMethod(name string) any {
	switch name {
	case "Do":
		return func(c *http.Client, req *http.Request) (*http.Response, error) { return st.via(c).Do(req) }
	case "Get":
		return func(c *http.Client, url string) (*http.Response, error) { return st.via(c).Get(url) }
	case "Head":
		return func(c *http.Client, url string) (*http.Response, error) { return st.via(c).Head(url) }
	case "Post":
		return func(c *http.Client, url, contentType string, body io.Reader) (*http.Response, error) {
			return st.via(c).Post(url, contentType, body)
		}
	case "PostForm":
		return func(c *http.Client, url string, data url.Values) (*http.Response, error) {
			return st.via(c).PostForm(url, data)
		}
	case "CloseIdleConnections":
		// With a nil Transport, Go closes the idle connections of
		// http.DefaultTransport: here the machine's, not the host's.
		return func(c *http.Client) { st.via(c).CloseIdleConnections() }
	}
	return nil
}

// proxyServeHTTP is (*httputil.ReverseProxy).ServeHTTP for the machine: a
// nil Transport sends through the machine's DefaultTransport, as in Go it
// means the current http.DefaultTransport. A request that the plugin sent
// with its DefaultClient and then gives to the proxy goes through the same
// transport: since Go 1.27, the transport sends a body that was already
// read again with GetBody when it retries on a kept-alive connection.
func (st *httpDefaults) proxyServeHTTP(p *httputil.ReverseProxy, w http.ResponseWriter, r *http.Request) {
	if p == nil || p.Transport != nil {
		p.ServeHTTP(w, r) // a nil p panics as in Go
		return
	}
	cp := *p
	cp.Transport = defaultTransport{st}
	if st.transport != nil {
		cp.Transport = st.transport
	}
	cp.ServeHTTP(w, r)
}

// muxGET is the method prefix of the routes that expvar and net/http/pprof
// register: "GET " unless GODEBUG=httpmuxgo121=1. The host's expvar (linked
// by stdsyms/sideeffect, which a program that imports expvar or
// net/http/pprof needs) registered its route with the right one.
var muxGET = sync.OnceValue(func() string {
	_, p := http.DefaultServeMux.Handler(&http.Request{Method: "GET", URL: &url.URL{Path: "/debug/vars"}})
	if strings.HasPrefix(p, "GET ") {
		return "GET "
	}
	return ""
})

// or returns h, or the machine's DefaultServeMux for a nil h.
func (st *httpDefaults) or(h http.Handler) http.Handler {
	if h == nil {
		return st.handler
	}
	return h
}

// serverFillMu guards the Handler field that fillHandler sets: two Serve
// calls on the same server can run at the same time.
var serverFillMu sync.Mutex

// fillHandler gives s the machine's DefaultServeMux when its Handler is
// nil, before a method of s serves. It also records s (startedServers): a
// nil Handler that the plugin stores later gets the same replacement.
func (m *Machine) fillHandler(s *http.Server) {
	st := m.httpDefaults()
	serverFillMu.Lock()
	defer serverFillMu.Unlock()
	if s.Handler == nil {
		s.Handler = st.handler
	}
	key := uintptr(unsafe.Pointer(&s.Handler))
	if old, ok := startedServers.Load(key); ok && old.(startedServer).s.Value() == s {
		return
	}
	e := startedServer{weak.Make(s), st}
	startedServers.Store(key, e)
	runtime.AddCleanup(s, func(k startedKey) { startedServers.CompareAndDelete(k.key, k.e) }, startedKey{key, e})
}

// startedServers holds the servers that a machine started (fillHandler),
// by the address of their Handler field: net/http reads the field at each
// request, and serves the host's http.DefaultServeMux when it is nil. So a
// plugin that sets the Handler of its running server to nil reached the
// routes of the host. setNative stores the machine's DefaultServeMux
// instead (lateNilHandler), as a nil Handler means in Go for the plugin.
var startedServers sync.Map // uintptr -> startedServer

// startedServer is an entry of startedServers. The weak pointer checks that
// the address is still the field of that server (memory is reused after the
// GC), and lets the GC free the server.
type startedServer struct {
	s  weak.Pointer[http.Server]
	st *httpDefaults
}

// startedKey is the argument of the cleanup that removes an entry.
type startedKey struct {
	key uintptr
	e   startedServer
}

var (
	handlerType = reflect.TypeFor[http.Handler]()
	serverType  = reflect.TypeFor[http.Server]()
)

// lateNilServer returns rv, the http.Server value that the plugin stores
// at dst, with the replacement of lateNilHandler for a nil Handler when
// dst is a server that a machine started.
func lateNilServer(dst, rv reflect.Value) reflect.Value {
	s := (*http.Server)(dst.Addr().UnsafePointer())
	h := lateNilHandler(unsafe.Pointer(&s.Handler))
	if h == nil || !rv.FieldByName("Handler").IsNil() {
		return rv
	}
	c := reflect.New(serverType).Elem()
	c.Set(rv)
	c.Addr().Interface().(*http.Server).Handler = h
	return c
}

// lateNilHandler returns the handler to store in place of nil at the
// address p, when p is the Handler field of a server that a machine started
// (startedServers), or nil.
func lateNilHandler(p unsafe.Pointer) http.Handler {
	v, ok := startedServers.Load(uintptr(p))
	if !ok {
		return nil
	}
	e := v.(startedServer)
	if s := e.s.Value(); s == nil || unsafe.Pointer(&s.Handler) != p {
		return nil
	}
	return e.st.handler
}

// httpFunc returns the machine's version of the function name of net/http.
func (m *Machine) httpFunc(name string) any {
	st := m.httpDefaults()
	switch name {
	case "Handle":
		return func(pattern string, h http.Handler) { st.mux.Handle(pattern, h) }
	case "HandleFunc":
		return func(pattern string, h func(http.ResponseWriter, *http.Request)) { st.mux.HandleFunc(pattern, h) }
	case "ListenAndServe":
		return func(addr string, h http.Handler) error { return http.ListenAndServe(addr, st.or(h)) }
	case "ListenAndServeTLS":
		return func(addr, cert, key string, h http.Handler) error {
			return http.ListenAndServeTLS(addr, cert, key, st.or(h))
		}
	case "Serve":
		return func(l net.Listener, h http.Handler) error { return http.Serve(l, st.or(h)) }
	case "ServeTLS":
		return func(l net.Listener, h http.Handler, cert, key string) error {
			return http.ServeTLS(l, st.or(h), cert, key)
		}
	case "Get":
		return func(url string) (*http.Response, error) { return st.via(st.client).Get(url) }
	case "Head":
		return func(url string) (*http.Response, error) { return st.via(st.client).Head(url) }
	case "Post":
		return func(url, contentType string, body io.Reader) (*http.Response, error) {
			return st.via(st.client).Post(url, contentType, body)
		}
	case "PostForm":
		return func(url string, data url.Values) (*http.Response, error) {
			return st.via(st.client).PostForm(url, data)
		}
	}
	return nil
}

// httpServeFunc returns the machine's version of the functions of
// httptest, fcgi and cgi that serve http.DefaultServeMux for a nil handler.
func (m *Machine) httpServeFunc(pkg, name string) any {
	st := m.httpDefaults()
	switch pkg + "." + name {
	case "net/http/httptest.NewServer":
		return func(h http.Handler) *httptest.Server {
			ts := httptest.NewServer(st.or(h))
			m.fillHandler(ts.Config) // records the server (lateNilHandler)
			return ts
		}
	case "net/http/httptest.NewTLSServer":
		return func(h http.Handler) *httptest.Server {
			ts := httptest.NewTLSServer(st.or(h))
			m.fillHandler(ts.Config)
			return ts
		}
	case "net/http/httptest.NewUnstartedServer":
		return func(h http.Handler) *httptest.Server { return httptest.NewUnstartedServer(st.or(h)) }
	case "net/http/fcgi.Serve":
		return func(l net.Listener, h http.Handler) error { return fcgi.Serve(l, st.or(h)) }
	case "net/http/cgi.Serve":
		return func(h http.Handler) error { return cgi.Serve(st.or(h)) }
	}
	return nil
}

// curHTTP returns the net/http variables of the machine of the goroutine
// for the AOT copies, or nil for a machine with the standard library as in
// Go (the host's variables). A copy that runs on a goroutine of no machine
// panics: it must not reach the host's mux.
func curHTTP(fn string) *httpDefaults {
	m := curMachine()
	if m == nil {
		panic(fmt.Sprintf("vm: net/http.%s called by an AOT copy outside of a plugin goroutine", fn))
	}
	if m.realStd || !m.native("net/http") {
		return nil
	}
	return m.httpDefaults()
}

// AOTHandle replaces http.Handle in the AOT copies (cmd/aotgen): it
// registers on the DefaultServeMux of the machine of the goroutine.
func AOTHandle(pattern string, h http.Handler) {
	if st := curHTTP("Handle"); st != nil {
		st.mux.Handle(pattern, h)
		return
	}
	http.Handle(pattern, h)
}

// AOTHandleFunc replaces http.HandleFunc in the AOT copies.
func AOTHandleFunc(pattern string, h func(http.ResponseWriter, *http.Request)) {
	if st := curHTTP("HandleFunc"); st != nil {
		st.mux.HandleFunc(pattern, h)
		return
	}
	http.HandleFunc(pattern, h)
}

// AOTDefaultServeMux replaces the reads of http.DefaultServeMux in the AOT
// copies.
func AOTDefaultServeMux() *http.ServeMux {
	if st := curHTTP("DefaultServeMux"); st != nil {
		return st.mux
	}
	return http.DefaultServeMux
}

// aotHandler is the handler that a nil handler means in the AOT copies.
func aotHandler(fn string, h http.Handler) http.Handler {
	if st := curHTTP(fn); st != nil {
		return st.or(h)
	}
	return h
}

// AOTListenAndServe replaces http.ListenAndServe in the AOT copies.
func AOTListenAndServe(addr string, h http.Handler) error {
	return http.ListenAndServe(addr, aotHandler("ListenAndServe", h))
}

// AOTListenAndServeTLS replaces http.ListenAndServeTLS in the AOT copies.
func AOTListenAndServeTLS(addr, cert, key string, h http.Handler) error {
	return http.ListenAndServeTLS(addr, cert, key, aotHandler("ListenAndServeTLS", h))
}

// AOTServe replaces http.Serve in the AOT copies.
func AOTServe(l net.Listener, h http.Handler) error {
	return http.Serve(l, aotHandler("Serve", h))
}

// AOTServeTLS replaces http.ServeTLS in the AOT copies.
func AOTServeTLS(l net.Listener, h http.Handler, cert, key string) error {
	return http.ServeTLS(l, aotHandler("ServeTLS", h), cert, key)
}

// ---- expvar ----

// expvarSyms is the native table of package expvar, or nil when the host
// did not import stdsyms/sideeffect. Package vm does not import expvar:
// its init registers /debug/vars on the host's http.DefaultServeMux.
func expvarSyms() map[string]reflect.Value { return stdsyms.Symbols["expvar/expvar"] }

// expvarType returns the type name of package expvar.
func expvarType(name string) reflect.Type { return expvarSyms()[name].Type().Elem() }

// expvarState is the table of expvar variables of one machine.
type expvarState struct {
	mu   sync.Mutex
	vars map[string]reflect.Value // expvar.Var values
	keys []string                 // sorted, as the table of package expvar
}

func (m *Machine) expvars() *expvarState {
	r := m.hostRegs()
	r.expvarOnce.Do(func() {
		st := &expvarState{vars: map[string]reflect.Value{}}
		// A restricted plugin starts with an empty table: the command line
		// and the memory statistics are those of the host.
		if !m.restricted {
			get := expvarSyms()["Get"]
			for _, name := range []string{"cmdline", "memstats"} { // published by expvar's init
				if v := get.Call([]reflect.Value{reflect.ValueOf(name)})[0]; !v.IsNil() {
					st.vars[name] = v
					st.keys = append(st.keys, name)
				}
			}
		}
		r.expvar = st
	})
	return r.expvar
}

// expvarPublish is expvar.Publish for m; v is an expvar.Var.
func (m *Machine) expvarPublish(name string, v reflect.Value) {
	st := m.expvars()
	st.mu.Lock()
	_, dup := st.vars[name]
	if !dup {
		st.vars[name] = v.Convert(expvarType("Var"))
		i, _ := slices.BinarySearch(st.keys, name)
		st.keys = slices.Insert(st.keys, i, name)
	}
	st.mu.Unlock()
	if dup {
		m.logPanicln("Reuse of exported var name:", name)
	}
}

// expvarDo calls f for each variable of m, in the order of the names.
func (m *Machine) expvarDo(f func(name string, v reflect.Value)) {
	st := m.expvars()
	st.mu.Lock()
	keys := slices.Clone(st.keys)
	vars := maps.Clone(st.vars)
	st.mu.Unlock()
	for _, k := range keys {
		f(k, vars[k])
	}
}

// expvarHandler serves the machine's variables as expvar's handler does.
func (m *Machine) expvarHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		b := []byte("{\n")
		first := true
		key := reflect.New(expvarType("String")) // the same JSON quoting as expvar
		m.expvarDo(func(name string, v reflect.Value) {
			if !first {
				b = append(b, ",\n"...)
			}
			first = false
			key.MethodByName("Set").Call([]reflect.Value{reflect.ValueOf(name)})
			b = append(b, key.Interface().(fmt.Stringer).String()...)
			b = append(b, ": "...)
			if v.IsNil() {
				b = append(b, "null"...)
			} else {
				b = append(b, v.Interface().(fmt.Stringer).String()...)
			}
		})
		b = append(b, "\n}\n"...)
		w.Write(b)
	})
}

// expvarFunc returns the machine's version of the function name of expvar,
// with the type of the native function, or an invalid value.
func (m *Machine) expvarFunc(name string) reflect.Value {
	syms := expvarSyms()
	orig, ok := syms[name]
	if !ok {
		return reflect.Value{}
	}
	// newVar returns a new variable of the type typ of expvar, published as
	// name.
	newVar := func(typ string) func([]reflect.Value) []reflect.Value {
		return func(in []reflect.Value) []reflect.Value {
			v := reflect.New(expvarType(typ))
			if typ == "Map" {
				v.MethodByName("Init").Call(nil)
			}
			m.expvarPublish(in[0].String(), v)
			return []reflect.Value{v}
		}
	}
	var impl func([]reflect.Value) []reflect.Value
	switch name {
	case "Publish":
		impl = func(in []reflect.Value) []reflect.Value {
			m.expvarPublish(in[0].String(), in[1])
			return nil
		}
	case "Get":
		impl = func(in []reflect.Value) []reflect.Value {
			st := m.expvars()
			st.mu.Lock()
			v, ok := st.vars[in[0].String()]
			st.mu.Unlock()
			if !ok {
				v = reflect.Zero(expvarType("Var"))
			}
			return []reflect.Value{v}
		}
	case "Do":
		impl = func(in []reflect.Value) []reflect.Value {
			kvType := expvarType("KeyValue")
			m.expvarDo(func(name string, v reflect.Value) {
				kv := reflect.New(kvType).Elem()
				kv.FieldByName("Key").SetString(name)
				kv.FieldByName("Value").Set(v)
				in[0].Call([]reflect.Value{kv})
			})
			return nil
		}
	case "Handler":
		return reflect.ValueOf(m.expvarHandler)
	case "NewInt":
		impl = newVar("Int")
	case "NewFloat":
		impl = newVar("Float")
	case "NewMap":
		impl = newVar("Map")
	case "NewString":
		impl = newVar("String")
	default:
		return reflect.Value{}
	}
	return reflect.MakeFunc(orig.Type(), impl)
}

// logPanicln is log.Panicln of the machine's default logger (stdio.go).
func (m *Machine) logPanicln(a ...any) {
	if fn, ok := m.overrides["log.Panicln"]; ok {
		fn.CallSlice([]reflect.Value{reflect.ValueOf(a)})
	}
	panic(fmt.Sprintln(a...))
}

// ---- database/sql ----

// sqlPrefixTag starts the names under which the machines register their
// sql drivers in the table of the process.
const sqlPrefixTag = "yaegi\x00"

// sqlSeq numbers the machines that register sql drivers.
var sqlSeq atomic.Int64

// sqlState holds the sql drivers of one machine.
type sqlState struct {
	mu      sync.Mutex
	prefix  string                   // AOT: the names in the table of the copy
	drivers map[string]driver.Driver // native: the drivers; AOT: nil values
}

func (m *Machine) sqlDrivers() *sqlState {
	r := m.hostRegs()
	r.sqlOnce.Do(func() {
		r.sql = &sqlState{prefix: fmt.Sprintf("%s%d\x00", sqlPrefixTag, sqlSeq.Add(1)), drivers: map[string]driver.Driver{}}
	})
	return r.sql
}

// register adds the driver d of name for sql.Register. It panics like
// sql.Register on a second registration.
func (st *sqlState) register(name string, d driver.Driver) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if _, dup := st.drivers[name]; dup {
		panic("sql: Register called twice for driver " + name)
	}
	st.drivers[name] = d
}

// lookup returns the driver of the machine for name.
func (st *sqlState) lookup(name string) (driver.Driver, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	d, ok := st.drivers[name]
	return d, ok
}

// names returns sql.Drivers for the machine: the drivers of the table of
// the process (without the names of the machines) and its own, sorted.
func (st *sqlState) names(all []string) []string {
	out := []string{}
	for _, d := range all {
		if !strings.HasPrefix(d, sqlPrefixTag) {
			out = append(out, d)
		}
	}
	st.mu.Lock()
	out = append(out, slices.Collect(maps.Keys(st.drivers))...)
	st.mu.Unlock()
	slices.Sort(out)
	return slices.Compact(out)
}

// errUnknownDriver is the error of sql.Open for an unknown driver.
func errUnknownDriver(name string) error {
	return fmt.Errorf("sql: unknown driver %q (forgotten import?)", name)
}

// dsnConnector is the connector of sql.Open for a driver that does not
// implement driver.DriverContext, as in database/sql.
type dsnConnector struct {
	dsn string
	d   driver.Driver
}

func (c dsnConnector) Connect(context.Context) (driver.Conn, error) { return c.d.Open(c.dsn) }

func (c dsnConnector) Driver() driver.Driver { return c.d }

// sqlFunc returns the machine's version of the function name of the native
// database/sql. The drivers of the machine stay out of the table of the
// process: Open opens them as sql.Open does, with sql.OpenDB.
func (m *Machine) sqlFunc(name string) any {
	st := m.sqlDrivers()
	switch name {
	case "Register":
		return func(name string, d driver.Driver) {
			if d == nil {
				panic("sql: Register driver is nil")
			}
			st.register(name, d)
		}
	case "Open":
		return func(name, dsn string) (*sql.DB, error) {
			d, ok := st.lookup(name)
			if !ok {
				return sql.Open(name, dsn) // a driver of the host
			}
			if dc, ok := d.(driver.DriverContext); ok {
				c, err := dc.OpenConnector(dsn)
				if err != nil {
					return nil, err
				}
				return sql.OpenDB(c), nil
			}
			return sql.OpenDB(dsnConnector{dsn: dsn, d: d}), nil
		}
	case "Drivers":
		return func() []string { return st.names(sql.Drivers()) }
	}
	return nil
}

// sqlHook returns the machine's hook of the function name of the AOT copy
// of database/sql. The copy has its own table of drivers, which the host
// does not use but all AOT plugins share: a driver of the machine has a
// name of the machine there (st.prefix).
func (m *Machine) sqlHook(name string) NativeHook {
	st := m.sqlDrivers()
	switch name {
	case "Register":
		orig := origHook("database/sql.Register")
		return func(m *Machine, a []Value) Value {
			n, _ := a[0].R.(string)
			st.register(n, nil)
			return orig(m, []Value{{R: st.prefix + n}, a[1]})
		}
	case "Open":
		orig := origHook("database/sql.Open")
		return func(m *Machine, a []Value) Value {
			n, _ := a[0].R.(string)
			if strings.HasPrefix(n, sqlPrefixTag) { // the driver of another machine
				return Tuple(Value{}, m.ErrorVal(errUnknownDriver(n)))
			}
			if _, ok := st.lookup(n); ok {
				n = st.prefix + n
			}
			return orig(m, []Value{{R: n}, a[1]})
		}
	case "Drivers":
		fn := aotPkg("database/sql").Funcs["Drivers"]
		return func(m *Machine, _ []Value) Value {
			return Ret(m, st.names(fn.Call(nil)[0].Interface().([]string)))
		}
	}
	return nil
}

// ---- mime ----

// mimeState is the table of mime types that a machine added.
type mimeState struct {
	mu    sync.RWMutex
	types map[string]string   // extension -> type
	lower map[string]string   // lower case extension -> type
	exts  map[string][]string // type without parameters -> lower case extensions
}

// add is mime.AddExtensionType on the machine's table.
func (st *mimeState) add(ext, typ string) error {
	if !strings.HasPrefix(ext, ".") {
		return fmt.Errorf("mime: extension %q missing leading dot", ext)
	}
	justType, param, err := mime.ParseMediaType(typ)
	if err != nil {
		return err
	}
	if strings.HasPrefix(typ, "text/") && param["charset"] == "" {
		param["charset"] = "utf-8"
		typ = mime.FormatMediaType(typ, param)
	}
	lower := strings.ToLower(ext)
	st.mu.Lock()
	defer st.mu.Unlock()
	st.types[ext] = typ
	st.lower[lower] = typ
	if !slices.Contains(st.exts[justType], lower) {
		st.exts[justType] = append(st.exts[justType], lower)
	}
	return nil
}

// typeOf is mime.TypeByExtension: the machine's table, then the host's.
func (st *mimeState) typeOf(ext string) string {
	st.mu.RLock()
	typ, ok := st.types[ext]
	if !ok {
		typ, ok = st.lower[strings.ToLower(ext)]
	}
	st.mu.RUnlock()
	if ok {
		return typ
	}
	return mime.TypeByExtension(ext)
}

// extsOf is mime.ExtensionsByType: the host's extensions and the
// machine's, sorted.
func (st *mimeState) extsOf(typ string) ([]string, error) {
	justType, _, err := mime.ParseMediaType(typ)
	if err != nil {
		return nil, err
	}
	host, err := mime.ExtensionsByType(typ)
	if err != nil {
		return nil, err
	}
	st.mu.RLock()
	own := st.exts[justType]
	st.mu.RUnlock()
	if len(own) == 0 {
		return host, nil
	}
	out := append(host, own...)
	slices.Sort(out)
	return slices.Compact(out), nil
}

func (m *Machine) mimeFunc(name string) any {
	st := &m.hostRegs().mime
	switch name {
	case "AddExtensionType":
		return st.add
	case "TypeByExtension":
		return st.typeOf
	case "ExtensionsByType":
		return st.extsOf
	}
	return nil
}

// ---- image ----

// imageFormat is a format of image.RegisterFormat.
type imageFormat struct {
	name, magic  string
	decode       func(io.Reader) (image.Image, error)
	decodeConfig func(io.Reader) (image.Config, error)
}

// imageState is the list of image formats that a machine registered.
type imageState struct {
	mu      sync.Mutex
	formats []imageFormat
}

// peekReader is an io.Reader that can peek ahead, as image's reader.
type peekReader interface {
	io.Reader
	Peek(int) ([]byte, error)
}

// sniff returns the first format of the machine that matches r.
func (st *imageState) sniff(r peekReader) (imageFormat, bool) {
	st.mu.Lock()
	formats := st.formats
	st.mu.Unlock()
	for _, f := range formats {
		b, err := r.Peek(len(f.magic))
		if err == nil && imageMatch(f.magic, b) {
			return f, true
		}
	}
	return imageFormat{}, false
}

// imageMatch is the match of package image: '?' matches any byte.
func imageMatch(magic string, b []byte) bool {
	if len(magic) != len(b) {
		return false
	}
	for i, c := range b {
		if magic[i] != c && magic[i] != '?' {
			return false
		}
	}
	return true
}

func asPeekReader(r io.Reader) peekReader {
	if rr, ok := r.(peekReader); ok {
		return rr
	}
	return bufio.NewReader(r)
}

func (m *Machine) imageFunc(name string) any {
	st := &m.hostRegs().image
	switch name {
	case "RegisterFormat":
		return func(name, magic string, decode func(io.Reader) (image.Image, error), decodeConfig func(io.Reader) (image.Config, error)) {
			st.mu.Lock()
			// A new slice: sniff reads the old one without the lock.
			st.formats = append(slices.Clip(st.formats), imageFormat{name, magic, decode, decodeConfig})
			st.mu.Unlock()
		}
	case "Decode":
		// The host's formats come first: they were registered before (init).
		// A peek reader is not consumed when no host format matches.
		return func(r io.Reader) (image.Image, string, error) {
			rr := asPeekReader(r)
			img, name, err := image.Decode(rr)
			if !errors.Is(err, image.ErrFormat) || name != "" {
				return img, name, err
			}
			f, ok := st.sniff(rr)
			if !ok || f.decode == nil {
				return nil, "", image.ErrFormat
			}
			img, err = f.decode(rr)
			return img, f.name, err
		}
	case "DecodeConfig":
		return func(r io.Reader) (image.Config, string, error) {
			rr := asPeekReader(r)
			c, name, err := image.DecodeConfig(rr)
			if !errors.Is(err, image.ErrFormat) || name != "" {
				return c, name, err
			}
			f, ok := st.sniff(rr)
			if !ok || f.decodeConfig == nil {
				return image.Config{}, "", image.ErrFormat
			}
			c, err = f.decodeConfig(rr)
			return c, f.name, err
		}
	}
	return nil
}

// ---- crypto, crypto/x509, archive/zip ----

// hash returns the hash function that the machine registered for h.
func (r *hostRegs) hash(h crypto.Hash) func() hash.Hash {
	r.mu2.Lock()
	defer r.mu2.Unlock()
	return r.hashes[h]
}

// cryptoZipFunc returns the machine's version of crypto.RegisterHash,
// x509.SetFallbackRoots and the functions of archive/zip that use the
// compressors and decompressors of the process.
func (m *Machine) cryptoZipFunc(pkg, name string) any {
	r := m.hostRegs()
	switch pkg + "." + name {
	case "crypto.RegisterHash":
		// (crypto.Hash).New and Available read the machine's table first
		// (hostMethod); the host's TLS and x509 checks keep the host's.
		return func(h crypto.Hash, f func() hash.Hash) {
			if h > crypto.BLAKE2b_512 { // maxHash of package crypto
				panic("crypto: RegisterHash of unknown hash function")
			}
			r.mu2.Lock()
			defer r.mu2.Unlock()
			if r.hashes == nil {
				r.hashes = map[crypto.Hash]func() hash.Hash{}
			}
			r.hashes[h] = f
		}
	case "crypto/x509.SetFallbackRoots":
		// The roots of the host stay: the native verification of the
		// plugin uses them too. The checks of Go are kept.
		return func(roots *x509.CertPool) {
			if roots == nil {
				panic("roots must be non-nil")
			}
			r.mu2.Lock()
			defer r.mu2.Unlock()
			if r.fallbackRoots {
				panic("SetFallbackRoots has already been called")
			}
			r.fallbackRoots = true
		}
	case "archive/zip.RegisterCompressor":
		return func(method uint16, c zip.Compressor) {
			r.mu2.Lock()
			defer r.mu2.Unlock()
			if _, dup := r.compressors[method]; dup || method == zip.Store || method == zip.Deflate {
				panic("compressor already registered")
			}
			if r.compressors == nil {
				r.compressors = map[uint16]zip.Compressor{}
			}
			r.compressors[method] = c
		}
	case "archive/zip.RegisterDecompressor":
		return func(method uint16, d zip.Decompressor) {
			r.mu2.Lock()
			defer r.mu2.Unlock()
			if _, dup := r.decompressors[method]; dup || method == zip.Store || method == zip.Deflate {
				panic("decompressor already registered")
			}
			if r.decompressors == nil {
				r.decompressors = map[uint16]zip.Decompressor{}
			}
			r.decompressors[method] = d
		}
	case "archive/zip.NewReader":
		return func(ra io.ReaderAt, size int64) (*zip.Reader, error) {
			zr, err := zip.NewReader(ra, size)
			if zr != nil {
				r.zipReader(zr)
			}
			return zr, err
		}
	case "archive/zip.OpenReader":
		return func(name string) (*zip.ReadCloser, error) {
			rc, err := zip.OpenReader(name)
			if rc != nil {
				r.zipReader(&rc.Reader)
			}
			return rc, err
		}
	case "archive/zip.NewWriter":
		return func(w io.Writer) *zip.Writer {
			zw := zip.NewWriter(w)
			r.mu2.Lock()
			defer r.mu2.Unlock()
			for method, c := range r.compressors {
				zw.RegisterCompressor(method, c)
			}
			return zw
		}
	}
	return nil
}

// zipReader gives zr the decompressors of the machine.
func (r *hostRegs) zipReader(zr *zip.Reader) {
	r.mu2.Lock()
	defer r.mu2.Unlock()
	for method, d := range r.decompressors {
		zr.RegisterDecompressor(method, d)
	}
}

// ---- net/rpc ----

// rpcServerVar returns the address of the machine's rpc.DefaultServer
// (native, or of the AOT copy), made at the first use.
func (m *Machine) rpcServerVar() reflect.Value {
	r := m.hostRegs()
	r.rpcOnce.Do(func() {
		newServer := m.pkgFunc("net/rpc", "NewServer")
		if !newServer.IsValid() {
			unsupported("net/rpc.NewServer not in registry")
		}
		s := newServer.Call(nil)[0]
		r.rpcServer = reflect.New(s.Type())
		r.rpcServer.Elem().Set(s)
	})
	return r.rpcServer
}

// rpcDefault returns the current value of the machine's native
// rpc.DefaultServer.
func (m *Machine) rpcDefault() *rpc.Server {
	return m.rpcServerVar().Elem().Interface().(*rpc.Server)
}

// pkgFunc returns the function name of the package path for m: the AOT
// copy, or the native one.
func (m *Machine) pkgFunc(pkgPath, name string) reflect.Value {
	if t := m.table(pkgPath); t != nil {
		if rv, ok := t.Funcs[name]; ok {
			return rv
		}
	}
	return m.stdTableKey(pkgPath + "/" + path.Base(pkgPath))[name]
}

// rpcHandleHTTP is (*rpc.Server).HandleHTTP for m: it registers on the
// machine's DefaultServeMux. The debug page needs the unexported handler
// type of net/rpc: its route answers 501.
func (m *Machine) rpcHandleHTTP(s *rpc.Server, rpcPath, debugPath string) {
	st := m.httpDefaults()
	st.mux.Handle(rpcPath, s)
	st.mux.Handle(debugPath, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "rpc: the debug page is not available to yaegi v2 plugins in native mode", http.StatusNotImplemented)
	}))
}

// rpcMethods are the functions of net/rpc that call the method of the
// same name on DefaultServer.
var rpcMethods = []string{"Accept", "Register", "RegisterName", "ServeCodec", "ServeConn", "ServeRequest"}

// rpcFunc returns the machine's version of the function name of the
// native net/rpc.
func (m *Machine) rpcFunc(name string) any {
	switch name {
	case "HandleHTTP":
		return func() { m.rpcHandleHTTP(m.rpcDefault(), rpc.DefaultRPCPath, rpc.DefaultDebugPath) }
	case "Accept":
		return func(l net.Listener) { m.rpcDefault().Accept(l) }
	case "Register":
		return func(rcvr any) error { return m.rpcDefault().Register(rcvr) }
	case "RegisterName":
		return func(name string, rcvr any) error { return m.rpcDefault().RegisterName(name, rcvr) }
	case "ServeCodec":
		return func(c rpc.ServerCodec) { m.rpcDefault().ServeCodec(c) }
	case "ServeConn":
		return func(c io.ReadWriteCloser) { m.rpcDefault().ServeConn(c) }
	case "ServeRequest":
		return func(c rpc.ServerCodec) error { return m.rpcDefault().ServeRequest(c) }
	}
	return nil
}

// rpcHook returns the machine's hook of the function name of the AOT copy
// of net/rpc: the method of the same name on the machine's DefaultServer.
// The copy's HandleHTTP registers through AOTHandle.
func (m *Machine) rpcHook(name string) NativeHook {
	args := func(a []Value) []Value { return a }
	switch {
	case name == "HandleHTTP":
		args = func([]Value) []Value { return []Value{{R: rpc.DefaultRPCPath}, {R: rpc.DefaultDebugPath}} }
	case !slices.Contains(rpcMethods, name):
		return nil
	}
	meth := origHook("(*net/rpc.Server)." + name)
	cell := m.rpcServerVar()
	return func(m *Machine, a []Value) Value {
		return meth(m, append([]Value{m.retVM(cell.Elem())}, args(a)...))
	}
}
