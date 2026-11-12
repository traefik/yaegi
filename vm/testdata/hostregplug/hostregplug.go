// Package hostregplug writes to one process-wide registry of the standard
// library, chosen by Config.Kind, when New is called: routes of
// http.DefaultServeMux, a database/sql driver, a mime
// type, an image format, a net/rpc service (and its HandleHTTP routes),
// fields of http.DefaultClient and http.DefaultTransport, a crypto hash,
// the x509 fallback roots, or a zip compressor and decompressor. Two instances of
// the plugin must not collide, and the host must see none of it. Its
// handler reports what the plugin sees, one line per check. The expvar and
// net/http/pprof cases are in hostregvarplug: restricted plugins may not
// import these packages.
package hostregplug

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto"
	"crypto/x509"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"hash"
	"image"
	"image/color"
	"io"
	"mime"
	"net"
	"net/http"
	"net/http/httptest"
	"net/rpc"
	"slices"
	"strings"
	"time"
)

type Config struct {
	Kind string
}

func CreateConfig() *Config { return &Config{} }

type drv struct{}

func (drv) Open(name string) (driver.Conn, error) { return nil, errors.New("hostreg: no conn " + name) }

type Arith int

type Args struct{ A, B int }

func (*Arith) Add(args *Args, reply *int) error {
	*reply = args.A + args.B
	return nil
}

// fakeHash is the SHA-256 of the plugin: a hash of size 7.
type fakeHash struct{}

func (fakeHash) Write(p []byte) (int, error) { return len(p), nil }
func (fakeHash) Sum(b []byte) []byte         { return append(b, "fake"...) }
func (fakeHash) Reset()                      {}
func (fakeHash) Size() int                   { return 7 }
func (fakeHash) BlockSize() int              { return 1 }

// upper is the zip method 99 of the plugin: it stores the data in upper
// case and reads it back in lower case.
type upper struct{ w io.Writer }

func (u upper) Write(p []byte) (int, error) { return u.w.Write(bytes.ToUpper(p)) }
func (u upper) Close() error                { return nil }

var rpcErr error

func New(_ context.Context, _ http.Handler, cfg *Config, _ string) (http.Handler, error) {
	switch cfg.Kind {
	case "mux":
		http.HandleFunc("/hostreg", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "plugin route") })
		http.Handle("/hostreg/handle", http.NotFoundHandler())
		http.DefaultServeMux.HandleFunc("/hostreg/mux", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "plugin mux") })
	case "sql":
		sql.Register("hostregdrv", drv{})
	case "mime":
		if err := mime.AddExtensionType(".hostreg", "text/x-hostreg"); err != nil {
			return nil, err
		}
	case "image":
		image.RegisterFormat("hostreg", "HOSTREG", decode, decodeConfig)
	case "rpc":
		rpcErr = rpc.Register(new(Arith))
		rpc.HandleHTTP()
	case "client":
		http.DefaultClient.Timeout = 42 * time.Second
		http.DefaultTransport.(*http.Transport).MaxIdleConnsPerHost = 77
	case "crypto":
		crypto.RegisterHash(crypto.SHA256, func() hash.Hash { return fakeHash{} })
	case "x509":
		x509.SetFallbackRoots(x509.NewCertPool())
	case "zip":
		zip.RegisterCompressor(99, func(w io.Writer) (io.WriteCloser, error) { return upper{w}, nil })
		zip.RegisterDecompressor(99, func(r io.Reader) io.ReadCloser {
			b, _ := io.ReadAll(r)
			return io.NopCloser(bytes.NewReader(bytes.ToLower(b)))
		})
	default:
		return nil, fmt.Errorf("unknown kind %q", cfg.Kind)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serve(cfg.Kind, w, r)
	}), nil
}

func decode(r io.Reader) (image.Image, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return image.NewGray(image.Rect(0, 0, len(b), 1)), nil
}

func decodeConfig(io.Reader) (image.Config, error) {
	return image.Config{ColorModel: color.GrayModel, Width: 7, Height: 1}, nil
}

// serve writes one line per check of kind. The X-Host-Path header is a
// route of the host mux, which the plugin must not see.
func serve(kind string, w http.ResponseWriter, r *http.Request) {
	line := func(format string, a ...any) { fmt.Fprintf(w, format+"\n", a...) }
	switch kind {
	case "mux":
		rec := httptest.NewRecorder()
		http.DefaultServeMux.ServeHTTP(rec, httptest.NewRequest("GET", "/hostreg/mux", nil))
		line("mux: %s", rec.Body)
		line("host route: %q", pattern(r.Header.Get("X-Host-Path")))
		srv := httptest.NewServer(nil)
		line("httptest.NewServer(nil): %s", get(srv.URL+"/hostreg"))
		srv.Close()
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			line("listen: %v", err)
			return
		}
		go http.Serve(l, nil)
		line("http.Serve(l, nil): %s", get("http://"+l.Addr().String()+"/hostreg"))
		l.Close()
		if l, err = net.Listen("tcp", "127.0.0.1:0"); err != nil {
			line("listen: %v", err)
			return
		}
		s := &http.Server{}
		go s.Serve(l)
		line("Server{}.Serve: %s", get("http://"+l.Addr().String()+"/hostreg"))
		s.Close()
		// A *httptest.Server with a nil Config.Handler: Start calls the
		// native (*http.Server).Serve.
		if l, err = net.Listen("tcp", "127.0.0.1:0"); err != nil {
			line("listen: %v", err)
			return
		}
		ts := &httptest.Server{Listener: l, Config: &http.Server{}}
		ts.Start()
		line("httptest.Server{}.Start: %s", get(ts.URL+"/hostreg"))
		ts.Close()
		ts = httptest.NewUnstartedServer(http.NotFoundHandler())
		ts.Config.Handler = nil
		ts.Start()
		line("NewUnstartedServer, nil Handler, Start: %s", get(ts.URL+"/hostreg"))
		ts.Close()
		ts = httptest.NewUnstartedServer(http.NotFoundHandler())
		ts.Config.Handler = nil
		ts.StartTLS()
		line("NewUnstartedServer, nil Handler, StartTLS: %s", getWith(ts.Client(), ts.URL+"/hostreg"))
		ts.Close()
		if r.Header.Get("X-Race") != "" {
			// The checks below write the fields of a running server: data
			// races in Go, which the race detector reports (with Go 1.27,
			// at each new connection or kept one). go test -race leaves
			// them out; the other test runs check them.
			return
		}
		// A nil Handler set after the start: net/http reads the field at
		// each request, and nil means http.DefaultServeMux, which for a
		// plugin is its own.
		ts = httptest.NewServer(http.NotFoundHandler())
		ts.Config.Handler = nil
		line("late nil Handler: %s, host route: %s, nil: %v", get(ts.URL+"/hostreg"), get(ts.URL + r.Header.Get("X-Host-Path"))[:3], ts.Config.Handler == nil)
		ts.Close()
		if l, err = net.Listen("tcp", "127.0.0.1:0"); err != nil {
			line("listen: %v", err)
			return
		}
		s = &http.Server{Handler: http.NotFoundHandler()}
		go s.Serve(l)
		get("http://" + l.Addr().String() + "/hostreg") // the server runs
		hp := &s.Handler
		*hp = nil
		line("Server.Serve, late nil Handler: %s, nil: %v", get("http://"+l.Addr().String()+"/hostreg"), s.Handler == nil)
		// The whole struct (a data race in Go, but it must not reach the
		// host's routes either).
		s.Handler = http.NotFoundHandler()
		*s = http.Server{}
		line("Server.Serve, whole struct: %s", get("http://" + l.Addr().String() + r.Header.Get("X-Host-Path"))[:3])
		s.Close()
		l.Close() // the zero struct does not know its listener any more
	case "sql":
		db, err := sql.Open("hostregdrv", "dsn")
		if err != nil {
			line("sql.Open: %v", err)
		} else {
			_, isDrv := db.Driver().(drv)
			line("sql: %v %v", isDrv, db.Ping())
			db.Close()
		}
		line("sql.Drivers: %v", slices.Contains(sql.Drivers(), "hostregdrv"))
	case "mime":
		exts, err := mime.ExtensionsByType("text/x-hostreg")
		line("mime: %q %q %q %v", mime.TypeByExtension(".hostreg"), mime.TypeByExtension(".HOSTREG"), exts, err)
	case "image":
		img, format, err := image.Decode(strings.NewReader("HOSTREG data"))
		if err == nil {
			line("image.Decode: %s %v", format, img.Bounds())
		} else {
			line("image.Decode: %v", err)
		}
		cfg, format, err := image.DecodeConfig(strings.NewReader("HOSTREG data"))
		line("image.DecodeConfig: %s %d %v", format, cfg.Width, err)
	case "rpc":
		line("rpc route: %q", pattern(rpc.DefaultRPCPath))
		srv := httptest.NewServer(nil)
		client, err := rpc.DialHTTP("tcp", srv.Listener.Addr().String())
		if err != nil {
			line("rpc dial: %v", err)
		} else {
			var sum int
			err := client.Call("Arith.Add", &Args{2, 3}, &sum)
			line("rpc call: %d %v (register: %v)", sum, err, rpcErr)
			client.Close()
		}
		srv.Close()
	case "client":
		line("client: %v %d", http.DefaultClient.Timeout, http.DefaultTransport.(*http.Transport).MaxIdleConnsPerHost)
	case "crypto":
		line("crypto: %d %v", crypto.SHA256.New().Size(), crypto.SHA256.Available())
	case "x509":
		line("x509: registered")
	case "zip":
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		f, err := zw.CreateHeader(&zip.FileHeader{Name: "a", Method: 99})
		if err != nil {
			line("zip: %v", err)
			return
		}
		io.WriteString(f, "hello")
		zw.Close()
		raw := bytes.Contains(buf.Bytes(), []byte("HELLO"))
		zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
		if err != nil {
			line("zip: %v", err)
			return
		}
		rc, err := zr.File[0].Open()
		if err != nil {
			line("zip: %v", err)
			return
		}
		b, _ := io.ReadAll(rc)
		line("zip: %v %s", raw, b)
	}
}

func pattern(path string) string {
	_, p := http.DefaultServeMux.Handler(httptest.NewRequest("GET", path, nil))
	return p
}

func get(url string) string { return getWith(http.DefaultClient, url) }

func getWith(c *http.Client, url string) string {
	resp, err := c.Get(url)
	if err != nil {
		return err.Error()
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return fmt.Sprintf("%d %s", resp.StatusCode, b)
}
