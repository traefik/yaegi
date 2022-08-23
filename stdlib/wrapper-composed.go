package stdlib

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"reflect"
)

// Wrappers for composed interfaces which trigger a special behavior in stdlib.
// Note: it may become useless to pre-compile composed interface wrappers
// once golang/go#15924 is resolved.

// In net/http, a ResponseWriter may also implement a Hijacker.

type _netHTTPResponseWriterHijacker struct {
	IValue       interface{}
	WHeader      func() http.Header
	WWrite       func(a0 []byte) (int, error)
	WWriteHeader func(statusCode int)

	WHijack func() (net.Conn, *bufio.ReadWriter, error)
}

func (w _netHTTPResponseWriterHijacker) Header() http.Header {
	return w.WHeader()
}

func (w _netHTTPResponseWriterHijacker) Write(a0 []byte) (int, error) {
	return w.WWrite(a0)
}

func (w _netHTTPResponseWriterHijacker) WriteHeader(statusCode int) {
	w.WWriteHeader(statusCode)
}

func (w _netHTTPResponseWriterHijacker) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.WHijack()
}

// In io, a Reader may implement WriteTo, used by io.Copy().

type _ioReaderWriteTo struct {
	IValue interface{}
	WRead  func(p []byte) (n int, err error)

	WWriteTo func(w io.Writer) (n int64, err error)
}

func (w _ioReaderWriteTo) Read(p []byte) (n int, err error) {
	return w.WRead(p)
}

func (w _ioReaderWriteTo) WriteTo(wr io.Writer) (n int64, err error) {
	return w.WWriteTo(wr)
}

func init() {
	MapTypes[reflect.ValueOf((*_net_http_ResponseWriter)(nil))] = []reflect.Type{
		reflect.ValueOf((*_netHTTPResponseWriterHijacker)(nil)).Type().Elem(),
	}
	MapTypes[reflect.ValueOf((*_io_Reader)(nil))] = []reflect.Type{
		reflect.ValueOf((*_ioReaderWriteTo)(nil)).Type().Elem(),
	}
}
