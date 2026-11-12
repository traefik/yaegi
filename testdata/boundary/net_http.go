// Native boundary of net/http (client side, writes, ResponseController,
// files): program types given to net/http (docs/BOUNDARY.md).
package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
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

// ---- writers ----

// plainW is a writer with only Write: it logs the size of each call.
type plainW struct {
	sb    strings.Builder
	calls []string
}

func (w *plainW) Write(p []byte) (int, error) {
	w.calls = append(w.calls, fmt.Sprintf("W%d", len(p)))
	return w.sb.Write(p)
}

// strW has WriteString (pointer receivers).
type strW struct{ plainW }

func (w *strW) WriteString(s string) (int, error) {
	w.calls = append(w.calls, fmt.Sprintf("S%d", len(s)))
	return w.sb.WriteString(s)
}

// byteW has WriteByte (io.ByteWriter).
type byteW struct{ plainW }

func (w *byteW) WriteByte(c byte) error {
	w.calls = append(w.calls, "B")
	return w.sb.WriteByte(c)
}

// gStrW is a generic writer with WriteString (value receivers).
type gStrW[T any] struct {
	tag T
	sb  *strings.Builder
	n   *int
}

func (w gStrW[T]) Write(p []byte) (int, error) { *w.n += 1000; return w.sb.Write(p) }
func (w gStrW[T]) WriteString(s string) (int, error) {
	*w.n++
	return w.sb.WriteString(s)
}

func calls(c []string) string {
	if len(c) > 12 {
		return fmt.Sprintf("%s ... (%d calls)", strings.Join(c[:12], ","), len(c))
	}
	return strings.Join(c, ",")
}

func flat(s string) string { return strings.ReplaceAll(s, "\r\n", "|") }

// ---- bodies ----

// wtSrc is a reader with WriteTo (value receiver).
type wtSrc struct{ s string }

func (r wtSrc) Read(p []byte) (int, error) { out("wtSrc.Read"); return 0, io.EOF }
func (r wtSrc) WriteTo(w io.Writer) (int64, error) {
	n, err := io.WriteString(w, r.s)
	out("wtSrc.WriteTo %d", n)
	return int64(n), err
}

// gWtSrc is a generic reader with WriteTo.
type gWtSrc[T any] struct{ v T }

func (r gWtSrc[T]) Read(p []byte) (int, error) { out("gWtSrc.Read"); return 0, io.EOF }
func (r gWtSrc[T]) WriteTo(w io.Writer) (int64, error) {
	n, err := fmt.Fprintf(w, "g%v", r.v)
	out("gWtSrc.WriteTo %d", n)
	return int64(n), err
}

// plainSrc is a reader without WriteTo (pointer receiver).
type plainSrc struct{ s string }

func (r *plainSrc) Read(p []byte) (int, error) {
	if r.s == "" {
		return 0, io.EOF
	}
	n := copy(p, r.s)
	r.s = r.s[n:]
	return n, nil
}

// sliceErr is an error type that cannot be compared (a slice).
type sliceErr []string

func (e sliceErr) Error() string { return "slice error " + strings.Join(e, "+") }

// cmpErr is a comparable error type of the program.
type cmpErr struct{ s string }

func (e cmpErr) Error() string { return "cmp error " + e.s }

// errSrc gives some bytes, then its error.
type errSrc struct {
	data string
	err  error
}

func (r *errSrc) Read(p []byte) (int, error) {
	if r.data == "" {
		return 0, r.err
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}

// rcBody is a body with Close (pointer receivers).
type rcBody struct {
	r      *strings.Reader
	closed bool
}

func (b *rcBody) Read(p []byte) (int, error) { return b.r.Read(p) }
func (b *rcBody) Close() error               { b.closed = true; return nil }

// valBody is a body without Close (value receiver).
type valBody struct{ s *strings.Reader }

func (b valBody) Read(p []byte) (int, error) { return b.s.Read(p) }

// gBody is a generic body with Close.
type gBody[T any] struct {
	v T
	r *strings.Reader
}

func (b gBody[T]) Read(p []byte) (int, error) { return b.r.Read(p) }
func (b gBody[T]) Close() error               { return nil }

// ---- round trippers ----

// recRT is a RoundTripper of the program: it prints the request.
type recRT struct{ name string }

func (rt *recRT) RoundTrip(req *http.Request) (*http.Response, error) {
	body := "<nil>"
	if req.Body != nil {
		b, _ := io.ReadAll(req.Body)
		body = fmt.Sprintf("%q", b)
		req.Body.Close()
	}
	out("%s: %s %s body=%T %s len=%d ct=%q", rt.name, req.Method, req.URL, req.Body, body, req.ContentLength, req.Header.Get("Content-Type"))
	return &http.Response{StatusCode: 200, Status: "200 OK", Header: http.Header{}, Body: io.NopCloser(strings.NewReader("ok")), Request: req}, nil
}

// waitCancel waits for the old Request.Cancel channel: net/http sets it
// for the RoundTrippers that it does not know.
func waitCancel(name string, req *http.Request) {
	select {
	case <-req.Cancel:
		out("%s: Request.Cancel closed", name)
	case <-time.After(5 * time.Second):
		out("%s: Request.Cancel not closed", name)
	}
}

// cancelRT has CancelRequest (the old canceler interface).
type cancelRT struct {
	called chan struct{}
	path   string
	once   sync.Once
}

func (rt *cancelRT) RoundTrip(req *http.Request) (*http.Response, error) {
	waitCancel("cancelRT", req)
	select {
	case <-rt.called:
		out("cancelRT: CancelRequest %s", rt.path)
	case <-time.After(5 * time.Second):
		out("cancelRT: CancelRequest not called")
	}
	return nil, errors.New("canceled")
}

func (rt *cancelRT) CancelRequest(req *http.Request) {
	rt.once.Do(func() { rt.path = req.URL.Path; close(rt.called) })
}

// blockRT has no CancelRequest.
type blockRT struct{}

func (blockRT) RoundTrip(req *http.Request) (*http.Response, error) {
	waitCancel("blockRT", req)
	return nil, errors.New("stopped")
}

// gBlockRT is a generic RoundTripper.
type gBlockRT[T any] struct{ v T }

func (rt *gBlockRT[T]) RoundTrip(req *http.Request) (*http.Response, error) {
	waitCancel(fmt.Sprintf("gBlockRT[%v]", rt.v), req)
	return nil, errors.New("g stopped")
}

// Transport has the name of a known type of another package: only its
// package path makes the difference (*main.Transport).
type Transport struct{}

func (*Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	waitCancel("main.Transport", req)
	return nil, errors.New("main transport")
}

// idleRT has CloseIdleConnections.
type idleRT struct{ name string }

func (rt idleRT) RoundTrip(req *http.Request) (*http.Response, error) { return nil, io.EOF }
func (rt idleRT) CloseIdleConnections()                               { out("%s.CloseIdleConnections", rt.name) }

// gIdleRT is a generic RoundTripper with CloseIdleConnections.
type gIdleRT[T any] struct{ v T }

func (rt *gIdleRT[T]) RoundTrip(req *http.Request) (*http.Response, error) { return nil, io.EOF }
func (rt *gIdleRT[T]) CloseIdleConnections()                               { out("gIdleRT[%v].CloseIdleConnections", rt.v) }

// ---- response writers ----

// baseRW is a ResponseWriter of the program without any option.
type baseRW struct{ rec *httptest.ResponseRecorder }

func (w *baseRW) Header() http.Header         { return w.rec.Header() }
func (w *baseRW) Write(p []byte) (int, error) { return w.rec.Write(p) }
func (w *baseRW) WriteHeader(code int)        { w.rec.WriteHeader(code) }

// fullRW has every option of ResponseController.
type fullRW struct{ *baseRW }

func (w fullRW) FlushError() error { out("fullRW.FlushError"); return errors.New("flush failed") }
func (w fullRW) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	out("fullRW.Hijack")
	return nil, nil, errors.New("no conn")
}
func (w fullRW) SetReadDeadline(t time.Time) error {
	out("fullRW.SetReadDeadline zero=%v", t.IsZero())
	return nil
}
func (w fullRW) SetWriteDeadline(t time.Time) error {
	out("fullRW.SetWriteDeadline zero=%v", t.IsZero())
	return errors.New("write deadline")
}
func (w fullRW) EnableFullDuplex() error { out("fullRW.EnableFullDuplex"); return nil }

// flushRW has only Flush (http.Flusher).
type flushRW struct{ *baseRW }

func (w *flushRW) Flush() { out("flushRW.Flush") }

// wrapRW has Unwrap.
type wrapRW struct{ inner http.ResponseWriter }

func (w *wrapRW) Header() http.Header         { return w.inner.Header() }
func (w *wrapRW) Write(p []byte) (int, error) { return w.inner.Write(p) }
func (w *wrapRW) WriteHeader(code int)        { w.inner.WriteHeader(code) }
func (w *wrapRW) Unwrap() http.ResponseWriter { out("wrapRW.Unwrap"); return w.inner }

// gWrapRW is a generic ResponseWriter with Unwrap.
type gWrapRW[T any] struct {
	tag   T
	inner http.ResponseWriter
}

func (w gWrapRW[T]) Header() http.Header         { return w.inner.Header() }
func (w gWrapRW[T]) Write(p []byte) (int, error) { return w.inner.Write(p) }
func (w gWrapRW[T]) WriteHeader(code int)        { w.inner.WriteHeader(code) }
func (w gWrapRW[T]) Unwrap() http.ResponseWriter {
	out("gWrapRW[%v].Unwrap", w.tag)
	return w.inner
}

func rwVariants() []http.ResponseWriter {
	return []http.ResponseWriter{
		&baseRW{httptest.NewRecorder()},
		fullRW{&baseRW{httptest.NewRecorder()}},
		&flushRW{&baseRW{httptest.NewRecorder()}},
		&wrapRW{fullRW{&baseRW{httptest.NewRecorder()}}},
		gWrapRW[int]{1, &wrapRW{&flushRW{&baseRW{httptest.NewRecorder()}}}},
		&wrapRW{&baseRW{httptest.NewRecorder()}},
		httptest.NewRecorder(),
	}
}

func ctlErr(err error) string {
	return fmt.Sprintf("%v notSupported=%v", err, errors.Is(err, http.ErrNotSupported))
}

// ---- files ----

var modTime = time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)

// fileInfo is a fs.FileInfo of the program.
type fileInfo struct {
	name string
	size int64
	dir  bool
}

func (i fileInfo) Name() string { return i.name }
func (i fileInfo) Size() int64  { return i.size }
func (i fileInfo) Mode() fs.FileMode {
	if i.dir {
		return fs.ModeDir | 0o755
	}
	return 0o644
}
func (i fileInfo) ModTime() time.Time { return modTime }
func (i fileInfo) IsDir() bool        { return i.dir }
func (i fileInfo) Sys() any           { return nil }

// dirEntry is a fs.DirEntry of the program.
type dirEntry struct{ info fileInfo }

func (e dirEntry) Name() string               { return e.info.name }
func (e dirEntry) IsDir() bool                { return e.info.dir }
func (e dirEntry) Type() fs.FileMode          { return e.info.Mode().Type() }
func (e dirEntry) Info() (fs.FileInfo, error) { return e.info, nil }

// memFile is a file with Seek (fs.File and io.Seeker).
type memFile struct {
	name string
	r    *strings.Reader
}

func (f *memFile) Stat() (fs.FileInfo, error) {
	return fileInfo{name: f.name, size: f.r.Size()}, nil
}
func (f *memFile) Read(p []byte) (int, error) { return f.r.Read(p) }
func (f *memFile) Close() error               { return nil }
func (f *memFile) Seek(off int64, whence int) (int64, error) {
	return f.r.Seek(off, whence)
}

// noSeekFile is a file without Seek.
type noSeekFile struct{ r *strings.Reader }

func (f noSeekFile) Stat() (fs.FileInfo, error) {
	return fileInfo{name: "noseek.txt", size: f.r.Size()}, nil
}
func (f noSeekFile) Read(p []byte) (int, error) { return f.r.Read(p) }
func (f noSeekFile) Close() error               { return nil }

// rdDir is a directory with ReadDir (fs.ReadDirFile).
type rdDir struct {
	name  string
	files []string
}

func (d *rdDir) Stat() (fs.FileInfo, error) { return fileInfo{name: d.name, dir: true}, nil }
func (d *rdDir) Read(p []byte) (int, error) { return 0, errors.New("is a directory") }
func (d *rdDir) Close() error               { return nil }
func (d *rdDir) ReadDir(n int) ([]fs.DirEntry, error) {
	var es []fs.DirEntry
	for _, f := range d.files {
		es = append(es, dirEntry{fileInfo{name: f, size: 3}})
	}
	d.files = nil
	if len(es) == 0 && n > 0 {
		return nil, io.EOF
	}
	return es, nil
}

// noRdDir is a directory without ReadDir.
type noRdDir struct{ name string }

func (d noRdDir) Stat() (fs.FileInfo, error) { return fileInfo{name: d.name, dir: true}, nil }
func (d noRdDir) Read(p []byte) (int, error) { return 0, errors.New("is a directory") }
func (d noRdDir) Close() error               { return nil }

// memFS is a fs.FS of the program.
type memFS struct{}

func (memFS) Open(name string) (fs.File, error) {
	switch name {
	case ".", "dir":
		return &rdDir{name: name, files: []string{"b.txt", "a.txt", "<x>.txt"}}, nil
	case "nord":
		return noRdDir{"nord"}, nil
	case "dir/a.txt", "a.txt":
		return &memFile{"a.txt", strings.NewReader("hello file system")}, nil
	case "noseek.txt":
		return noSeekFile{strings.NewReader("no seek content")}, nil
	}
	return nil, fs.ErrNotExist
}

// httpFile is a http.File of the program with Readdir (and no ReadDir).
type httpFile struct {
	name string
	dir  bool
	r    *strings.Reader
}

func (f *httpFile) Close() error               { return nil }
func (f *httpFile) Read(p []byte) (int, error) { return f.r.Read(p) }
func (f *httpFile) Seek(off int64, whence int) (int64, error) {
	return f.r.Seek(off, whence)
}
func (f *httpFile) Readdir(n int) ([]fs.FileInfo, error) {
	if !f.dir {
		return nil, errors.New("not a dir")
	}
	return []fs.FileInfo{fileInfo{name: "z.txt", size: 1}, fileInfo{name: "y", dir: true}}, nil
}
func (f *httpFile) Stat() (fs.FileInfo, error) {
	return fileInfo{name: f.name, dir: f.dir, size: f.r.Size()}, nil
}

// rdHTTPFile also has ReadDir: dirList takes it.
type rdHTTPFile struct{ *httpFile }

func (f rdHTTPFile) ReadDir(n int) ([]fs.DirEntry, error) {
	return []fs.DirEntry{dirEntry{fileInfo{name: "readdir.txt", size: 2}}}, nil
}

// httpFS is a http.FileSystem of the program.
type httpFS struct{}

func (httpFS) Open(name string) (http.File, error) {
	switch name {
	case "/":
		return &httpFile{name: "/", dir: true, r: strings.NewReader("")}, nil
	case "/rd":
		return rdHTTPFile{&httpFile{name: "rd", dir: true, r: strings.NewReader("")}}, nil
	case "/f.txt":
		return &httpFile{name: "f.txt", r: strings.NewReader("http file")}, nil
	}
	return nil, fs.ErrNotExist
}

// showRec prints a recorded response (no Date header in a recorder).
func showRec(name string, rec *httptest.ResponseRecorder) {
	var hs []string
	for k, v := range rec.Header() {
		hs = append(hs, k+"="+strings.Join(v, ";"))
	}
	sort.Strings(hs)
	out("%s: %d [%s] %q", name, rec.Code, strings.Join(hs, " "), rec.Body.String())
}

// seekContent is a io.ReadSeeker of the program (value receiver over a pointer).
type seekContent struct{ r *strings.Reader }

func (c seekContent) Read(p []byte) (int, error) { return c.r.Read(p) }
func (c seekContent) Seek(off int64, whence int) (int64, error) {
	return c.r.Seek(off, whence)
}

// The server logs of net/http go to the default logger: no time in them.
func init() { log.SetFlags(0) }

func main() {
	// row: net/http Header.Write,Header.WriteSubset,(*Request).Write,(*Request).WriteProxy,(*Response).Write assert x.(io.StringWriter)
	probe("Header/StringWriter", func() {
		h := http.Header{"B": {"2", "3"}, "A": {"1"}, "C": {"x"}}
		p := &plainW{}
		out("Write plain: %v %s %q", h.Write(p), calls(p.calls), flat(p.sb.String()))
		s := &strW{}
		out("Write str: %v %s %q", h.Write(s), calls(s.calls), flat(s.sb.String()))
		n := 0
		g := gStrW[string]{"g", &strings.Builder{}, &n}
		out("Write gen: %v n=%d %q", h.Write(g), n, flat(g.sb.String()))
		type excl map[string]bool
		ex := excl{"B": true}
		s = &strW{}
		out("WriteSubset str: %v %s %q", h.WriteSubset(s, ex), calls(s.calls), flat(s.sb.String()))
		var w io.Writer = &strW{}
		out("WriteSubset iface: %v %s", h.WriteSubset(w, nil), calls(w.(*strW).calls))
		resp := &http.Response{StatusCode: 200, ProtoMajor: 1, ProtoMinor: 1, Header: http.Header{"X-A": {"a"}}, ContentLength: 0}
		s = &strW{}
		out("Response.Write str: %v %s %q", resp.Write(s), calls(s.calls), flat(s.sb.String()))
		p = &plainW{}
		out("Response.Write plain: %v %s", resp.Write(p), calls(p.calls))
		req, _ := http.NewRequest("GET", "http://example.com/x", nil)
		req.Header.Set("X-R", "r")
		s = &strW{}
		out("Request.Write str: %v %s", req.Write(s), calls(s.calls))
		s = &strW{}
		out("Request.WriteProxy str: %v %s %q", req.WriteProxy(s), calls(s.calls), flat(s.sb.String()))
	})
	// row: net/http (*Request).Write,(*Request).WriteProxy assert x.(io.ByteWriter)
	probe("Request/ByteWriter", func() {
		req, _ := http.NewRequest("POST", "http://example.com/p?q=1", strings.NewReader("body"))
		req.Header.Set("X-A", "a")
		p := &plainW{}
		out("plain: %v %s %q", req.Write(p), calls(p.calls), flat(p.sb.String()))
		req, _ = http.NewRequest("POST", "http://example.com/p?q=1", strings.NewReader("body"))
		req.Header.Set("X-A", "a")
		b := &byteW{}
		out("byte: %v %s %q", req.Write(b), calls(b.calls), flat(b.sb.String()))
		req, _ = http.NewRequest("GET", "http://example.com/p", nil)
		b = &byteW{}
		out("byte proxy: %v %s %q", req.WriteProxy(b), calls(b.calls), flat(b.sb.String()))
		var w io.Writer = &byteW{}
		req, _ = http.NewRequest("GET", "http://example.com/i", nil)
		out("byte iface: %v %s", req.Write(w), calls(w.(*byteW).calls))
		var bw io.Writer = bufio.NewWriter(&plainW{})
		out("bufio: %v", req.Write(bw))
	})
	// row: net/http (*Request).Write,(*Request).WriteProxy,(*Response).Write assert x.(io.Reader)
	// row: net/http (*Request).Write,(*Request).WriteProxy,(*Response).Write reflect walk
	// row: net/http (*Request).Write,(*Request).WriteProxy,(*Response).Write assert x.(T), T unnamed
	// row: net/http (*Request).Write,(*Request).WriteProxy,(*Response).Write panic x.(T) without ok, T unnamed: *runtime.TypeAssertionError
	probe("Write/NopCloser", func() {
		// The *[32768]byte of the copy buffer comes from a pool of
		// net/http: the body copy uses it.
		bodies := []io.Reader{wtSrc{"value-wt"}, gWtSrc[int]{42}, &plainSrc{"plain"}}
		for i, r := range bodies {
			n := int64(len("value-wt"))
			if i == 1 {
				n = 3
			} else if i == 2 {
				n = 5
			}
			req, _ := http.NewRequest("PUT", "http://example.com/u", nil)
			req.Body = io.NopCloser(r)
			req.ContentLength = n
			p := &plainW{}
			err := req.Write(p)
			out("req %d: %v %s %q", i, err, calls(p.calls), flat(p.sb.String()))
		}
		// Unknown length: chunked.
		req, _ := http.NewRequest("POST", "http://example.com/c", nil)
		req.Body = io.NopCloser(&plainSrc{"chunk"})
		req.ContentLength = -1
		var sb strings.Builder
		out("chunked: %v %q", req.WriteProxy(&sb), flat(sb.String()))
		// GET with an unknown length: net/http reads one byte to see.
		req, _ = http.NewRequest("GET", "http://example.com/g", nil)
		req.Body = io.NopCloser(&plainSrc{""})
		sb.Reset()
		out("probe empty: %v %q", req.Write(&sb), flat(sb.String()))
		for i, r := range []io.Reader{wtSrc{"resp-wt"}, &plainSrc{"resp-plain"}} {
			resp := &http.Response{StatusCode: 200, ProtoMajor: 1, ProtoMinor: 1, Header: http.Header{}, Body: io.NopCloser(r), ContentLength: -1}
			sb.Reset()
			out("resp %d: %v %q", i, resp.Write(&sb), flat(sb.String()))
		}
	})
	// row: net/http (*Request).Write,(*Request).WriteProxy,(*Response).Write panic x.(io.Reader) without ok: *runtime.TypeAssertionError
	probe("Write/NopCloser-nil", func() {
		// io.NopCloser(nil): the field is a nil io.Reader, the assertion
		// of net/http panics.
		for _, proxy := range []bool{false, true} {
			func() {
				defer func() {
					r := recover()
					out("request proxy=%v: %T %v", proxy, r, r)
				}()
				req, _ := http.NewRequest("POST", "http://example.com/n", nil)
				req.Body = io.NopCloser(nil)
				req.ContentLength = 3
				var sb strings.Builder
				if proxy {
					req.WriteProxy(&sb)
				} else {
					req.Write(&sb)
				}
			}()
		}
		func() {
			defer func() {
				r := recover()
				out("response: %T %v", r, r)
			}()
			resp := &http.Response{StatusCode: 200, ProtoMajor: 1, ProtoMinor: 1, Header: http.Header{}, Body: io.NopCloser(nil), ContentLength: 3}
			var sb strings.Builder
			resp.Write(&sb)
		}()
	})
	// row: net/http (*Request).Write,(*Request).WriteProxy panic == of two interfaces: comparing uncomparable type
	probe("Request/uncomparable-error", func() {
		req, _ := http.NewRequest("PUT", "http://example.com/e", nil)
		req.Body = io.NopCloser(&errSrc{"ab", cmpErr{"c"}})
		req.ContentLength = 4
		var sb strings.Builder
		err := req.Write(&sb)
		var ce cmpErr
		out("comparable: %v as=%v", err, errors.As(err, &ce))
		func() {
			defer func() {
				r := recover()
				out("uncomparable: %T %v", r, r)
			}()
			req, _ := http.NewRequest("PUT", "http://example.com/e", nil)
			req.Body = io.NopCloser(&errSrc{"ab", sliceErr{"x", "y"}})
			req.ContentLength = 4
			sb.Reset()
			err := req.WriteProxy(&sb)
			out("uncomparable: no panic: %v", err)
		}()
	})
	// row: net/http NewRequest,NewRequestWithContext,Get,Head,Post,PostForm,(*Client).Get,(*Client).Head,(*Client).Post,(*Client).PostForm assert x.(io.ReadCloser)
	probe("Client/body", func() {
		rt := &recRT{"rt"}
		c := &http.Client{Transport: rt}
		bodies := []io.Reader{&rcBody{r: strings.NewReader("rc")}, valBody{strings.NewReader("val")}, gBody[string]{"g", strings.NewReader("gen")}, strings.NewReader("sr"), nil}
		for i, b := range bodies {
			req, err := http.NewRequest("POST", fmt.Sprintf("http://example.com/%d", i), b)
			if err != nil {
				out("NewRequest: %v", err)
				continue
			}
			_, isRC := req.Body.(*rcBody)
			out("NewRequest %d: %T rc=%v len=%d getBody=%v", i, req.Body, isRC, req.ContentLength, req.GetBody != nil)
			resp, err := c.Do(req)
			if err == nil {
				resp.Body.Close()
			}
		}
		resp, err := c.Post("http://example.com/post", "text/plain", gBody[int]{1, strings.NewReader("posted")})
		out("Client.Post: %v %v", resp != nil, err)
		resp, err = c.PostForm("http://example.com/form", url.Values{"k": {"v"}})
		out("Client.PostForm: %v %v", resp != nil, err)
		resp, err = c.Get("http://example.com/get")
		out("Client.Get: %v %v", resp != nil, err)
		resp, err = c.Head("http://example.com/head")
		out("Client.Head: %v %v", resp != nil, err)
		old := http.DefaultClient.Transport
		http.DefaultClient.Transport = &recRT{"default"}
		resp, err = http.Post("http://example.com/dpost", "a/b", &rcBody{r: strings.NewReader("dp")})
		out("Post: %v %v", resp != nil, err)
		resp, err = http.PostForm("http://example.com/dform", url.Values{"a": {"1", "2"}})
		out("PostForm: %v %v", resp != nil, err)
		resp, err = http.Get("http://example.com/dget")
		out("Get: %v %v", resp != nil, err)
		resp, err = http.Head("http://example.com/dhead")
		out("Head: %v %v", resp != nil, err)
		http.DefaultClient.Transport = old
	})
	// row: net/http (*Client).Do,(*Client).Get,(*Client).Head,(*Client).Post,(*Client).PostForm,Get,Head,Post,PostForm assert x.(net/http.canceler)
	// row: net/http (*Client).Do,(*Client).Get,(*Client).Head,(*Client).Post,(*Client).PostForm,Get,Head,Post,PostForm reflect type name
	// row: net/http (*Client).Do,(*Client).Get,(*Client).Head,(*Client).Post,(*Client).PostForm,Get,Head,Post,PostForm reflect walk
	probe("Client/timeout", func() {
		const d = 20 * time.Millisecond
		show := func(name string, err error) {
			var ue *url.Error
			if errors.As(err, &ue) {
				out("%s: %v timeout=%v", name, err, ue.Timeout())
			} else {
				out("%s: %v", name, err)
			}
		}
		rts := []http.RoundTripper{&cancelRT{called: make(chan struct{})}, blockRT{}, &gBlockRT[int]{5}, &Transport{}}
		for i, rt := range rts {
			c := &http.Client{Transport: rt, Timeout: d}
			req, _ := http.NewRequest("GET", fmt.Sprintf("http://example.com/do%d", i), nil)
			_, err := c.Do(req)
			show(fmt.Sprintf("Do %d", i), err)
		}
		c := &http.Client{Transport: &cancelRT{called: make(chan struct{})}, Timeout: d}
		_, err := c.Get("http://example.com/get")
		show("Client.Get", err)
		c = &http.Client{Transport: blockRT{}, Timeout: d}
		_, err = c.Head("http://example.com/head")
		show("Client.Head", err)
		c = &http.Client{Transport: &cancelRT{called: make(chan struct{})}, Timeout: d}
		_, err = c.Post("http://example.com/post", "a/b", strings.NewReader("x"))
		show("Client.Post", err)
		c = &http.Client{Transport: &Transport{}, Timeout: d}
		_, err = c.PostForm("http://example.com/form", url.Values{"a": {"b"}})
		show("Client.PostForm", err)
		old := *http.DefaultClient
		http.DefaultClient.Timeout = d
		http.DefaultClient.Transport = &cancelRT{called: make(chan struct{})}
		_, err = http.Get("http://example.com/dget")
		show("Get", err)
		http.DefaultClient.Transport = &gBlockRT[string]{"h"}
		_, err = http.Head("http://example.com/dhead")
		show("Head", err)
		http.DefaultClient.Transport = &cancelRT{called: make(chan struct{})}
		_, err = http.Post("http://example.com/dpost", "a/b", nil)
		show("Post", err)
		http.DefaultClient.Transport = blockRT{}
		_, err = http.PostForm("http://example.com/dform", nil)
		show("PostForm", err)
		*http.DefaultClient = old
	})
	// row: net/http (*Client).CloseIdleConnections assert x.(net/http.closeIdler)
	probe("Client/CloseIdleConnections", func() {
		for i, rt := range []http.RoundTripper{idleRT{"value"}, &gIdleRT[string]{"g"}, blockRT{}, &http.Transport{}, nil} {
			c := &http.Client{Transport: rt}
			c.CloseIdleConnections()
			out("%d done", i)
		}
	})
	// row: net/http (*ResponseController).Flush assert x.(interface{FlushError() error})
	// row: net/http (*ResponseController).Flush assert x.(net/http.Flusher)
	// row: net/http (*ResponseController).Flush assert x.(net/http.rwUnwrapper)
	probe("ResponseController.Flush", func() {
		for i, w := range rwVariants() {
			out("%d %T: %s", i, w, ctlErr(http.NewResponseController(w).Flush()))
		}
	})
	// row: net/http (*ResponseController).Hijack assert x.(net/http.Hijacker)
	// row: net/http (*ResponseController).Hijack assert x.(net/http.rwUnwrapper)
	probe("ResponseController.Hijack", func() {
		for i, w := range rwVariants() {
			c, brw, err := http.NewResponseController(w).Hijack()
			out("%d: %v %v %s", i, c == nil, brw == nil, ctlErr(err))
		}
	})
	// row: net/http (*ResponseController).SetReadDeadline assert x.(interface{SetReadDeadline(time.Time) error})
	// row: net/http (*ResponseController).SetReadDeadline assert x.(net/http.rwUnwrapper)
	// row: net/http (*ResponseController).SetWriteDeadline assert x.(interface{SetWriteDeadline(time.Time) error})
	// row: net/http (*ResponseController).SetWriteDeadline assert x.(net/http.rwUnwrapper)
	probe("ResponseController.Deadlines", func() {
		for i, w := range rwVariants() {
			rc := http.NewResponseController(w)
			out("%d read: %s", i, ctlErr(rc.SetReadDeadline(time.Time{})))
			out("%d write: %s", i, ctlErr(rc.SetWriteDeadline(modTime)))
		}
	})
	// row: net/http (*ResponseController).EnableFullDuplex assert x.(interface{EnableFullDuplex() error})
	// row: net/http (*ResponseController).EnableFullDuplex assert x.(net/http.rwUnwrapper)
	probe("ResponseController.EnableFullDuplex", func() {
		for i, w := range rwVariants() {
			out("%d: %s", i, ctlErr(http.NewResponseController(w).EnableFullDuplex()))
		}
	})
	// row: net/http ServeFile,ServeFileFS assert x.(io/fs.ReadDirFile)
	probe("ServeFile/dirList", func() {
		get := func(path string, hdr ...string) *http.Request {
			r := httptest.NewRequest("GET", path, nil)
			for i := 0; i+1 < len(hdr); i += 2 {
				r.Header.Set(hdr[i], hdr[i+1])
			}
			return r
		}
		rec := httptest.NewRecorder()
		http.ServeFileFS(rec, get("/dir/"), memFS{}, "dir")
		showRec("FS dir", rec)
		rec = httptest.NewRecorder()
		http.ServeFileFS(rec, get("/nord/"), memFS{}, "nord")
		showRec("FS dir without ReadDir", rec)
		rec = httptest.NewRecorder()
		http.ServeFileFS(rec, get("/dir/a.txt", "Range", "bytes=6-9"), memFS{}, "dir/a.txt")
		showRec("FS file range", rec)
		rec = httptest.NewRecorder()
		http.ServeFileFS(rec, get("/noseek.txt"), memFS{}, "noseek.txt")
		showRec("FS file without Seek", rec)
		rec = httptest.NewRecorder()
		http.ServeFileFS(rec, get("/missing"), memFS{}, "missing")
		showRec("FS missing", rec)
		// FileServer over program file systems.
		for _, p := range []string{"/", "/rd/", "/f.txt"} {
			rec = httptest.NewRecorder()
			http.FileServer(httpFS{}).ServeHTTP(rec, get(p))
			showRec("FileServer "+p, rec)
		}
		rec = httptest.NewRecorder()
		http.FileServerFS(memFS{}).ServeHTTP(rec, get("/"))
		showRec("FileServerFS /", rec)
		rec = httptest.NewRecorder()
		http.ServeContent(rec, get("/c", "Range", "bytes=0-3"), "c.txt", modTime, seekContent{strings.NewReader("content of c")})
		showRec("ServeContent", rec)
		// ServeFile on a real directory (os files).
		dir, err := os.MkdirTemp("", "boundary-http")
		if err != nil {
			out("MkdirTemp: %v", err)
			return
		}
		defer os.RemoveAll(dir)
		for _, n := range []string{"b.txt", "a.txt"} {
			p := filepath.Join(dir, n)
			os.WriteFile(p, []byte("data "+n), 0o644)
			os.Chtimes(p, modTime, modTime)
		}
		os.Chtimes(dir, modTime, modTime)
		rec = httptest.NewRecorder()
		http.ServeFile(rec, get("/d/"), dir)
		showRec("ServeFile dir", rec)
		rec = httptest.NewRecorder()
		http.ServeFile(rec, get("/d/a.txt"), filepath.Join(dir, "a.txt"))
		showRec("ServeFile file", rec)
	})
}
