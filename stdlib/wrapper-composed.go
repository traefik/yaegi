package stdlib

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"reflect"
)

// Wrappers for composed interfaces which trigger a special behaviour in stdlib.
// Note: it may become useless to pre-compile composed interface wrappers
// once golang/go#15924 is resolved.

// In net/http, a ResponseWriter may also implement a Hijacker.

type _net_http_ResponseWriter_Hijacker struct {
	IValue       interface{}
	WHeader      func() http.Header
	WWrite       func(a0 []byte) (int, error)
	WWriteHeader func(statusCode int)

	WHijack func() (net.Conn, *bufio.ReadWriter, error)
}

func (W _net_http_ResponseWriter_Hijacker) Header() http.Header {
	return W.WHeader()
}
func (W _net_http_ResponseWriter_Hijacker) Write(a0 []byte) (int, error) {
	return W.WWrite(a0)
}
func (W _net_http_ResponseWriter_Hijacker) WriteHeader(statusCode int) {
	W.WWriteHeader(statusCode)
}
func (W _net_http_ResponseWriter_Hijacker) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return W.WHijack()
}

// In io, a Reader may implement WriteTo, used by io.Copy().

type _io_Reader_WriteTo struct {
	IValue interface{}
	WRead  func(p []byte) (n int, err error)

	WWriteTo func(w io.Writer) (n int64, err error)
}

func (W _io_Reader_WriteTo) Read(p []byte) (n int, err error) {
	return W.WRead(p)
}
func (W _io_Reader_WriteTo) WriteTo(w io.Writer) (n int64, err error) {
	return W.WWriteTo(w)
}

func init() {
	MapTypes[reflect.ValueOf((*_net_http_ResponseWriter)(nil))] = []reflect.Type{
		reflect.ValueOf((*_net_http_ResponseWriter_Hijacker)(nil)).Type().Elem(),
	}
	MapTypes[reflect.ValueOf((*_io_Reader)(nil))] = []reflect.Type{
		reflect.ValueOf((*_io_Reader_WriteTo)(nil)).Type().Elem(),
	}
}
