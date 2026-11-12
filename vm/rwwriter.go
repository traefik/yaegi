package vm

// Optional interfaces of http.ResponseWriter. In Go, a ResponseWriter often
// has more methods than its interface: http.Flusher (streaming, SSE, gRPC),
// http.Hijacker (websockets, used by httputil.ReverseProxy for upgrades),
// io.ReaderFrom, http.Pusher, http.CloseNotifier, and what
// http.ResponseController looks for (Unwrap, FlushError, SetReadDeadline,
// SetWriteDeadline, EnableFullDuplex). Native code finds them with type
// assertions (w.(http.Flusher)). A plugin that wraps the writer (logging,
// compression, header capture) defines these methods on its own type.
//
// The generic wrapper of http.ResponseWriter (stdsyms) has only Header,
// Write and WriteHeader, so native code did not see the other methods. Go
// cannot build a type with methods at run time (reflect.StructOf does not
// promote the methods of embedded interfaces), so the wrappers are static
// types: one per combination of the optional features below
// (zz_rwcombos.go, made by rwcombos_gen.go). Each combination embeds one
// small struct per feature, so each feature method is written once. The
// wrapper of an interpreted value has exactly the features of its dynamic
// type: a plugin type without Flush is not a Flusher, and a plugin type
// without SetWriteDeadline has no SetWriteDeadline method, as in Go.

//go:generate go run rwcombos_gen.go

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"reflect"
	"time"
)

// The optional features, one bit each. The methods that only
// http.ResponseController looks for (FlushError, SetReadDeadline,
// SetWriteDeadline, EnableFullDuplex, the "control" methods) have one bit
// each too: the wrapper has exactly the methods of the type, so a direct
// type assertion in native code (w.(interface{ SetWriteDeadline(time.Time)
// error })) fails when the type does not have the method, as in Go.
//
// One exception keeps the binary small (each wrapper type costs about 4 KB
// in a stripped binary, and all 1024 combinations cost 8 MB in yaegi): a type
// with ReadFrom, Push or CloseNotify (Push and CloseNotify are deprecated)
// and some, but not all, control methods gets the wrapper with the four
// control methods (see rwComboMask). A control method that the type does
// not have then does what http.ResponseController would do without it, so
// the result through http.ResponseController is still the one of Go; only a
// direct type assertion of a missing control method succeeds.
const (
	rwFlush            = 1 << iota // Flush()
	rwHijack                       // Hijack() (net.Conn, *bufio.ReadWriter, error)
	rwReadFrom                     // ReadFrom(io.Reader) (int64, error)
	rwPush                         // Push(string, *http.PushOptions) error
	rwCloseNotify                  // CloseNotify() <-chan bool
	rwUnwrap                       // Unwrap() http.ResponseWriter
	rwFlushError                   // FlushError() error
	rwSetReadDeadline              // SetReadDeadline(time.Time) error
	rwSetWriteDeadline             // SetWriteDeadline(time.Time) error
	rwEnableFullDuplex             // EnableFullDuplex() error

	rwNumFeat = iota

	rwCtlAll = rwFlushError | rwSetReadDeadline | rwSetWriteDeadline | rwEnableFullDuplex
	rwRare   = rwReadFrom | rwPush | rwCloseNotify
)

// The methods of the features, in bit order (bit i is rwFeatMethods[i]).
var (
	rwFeatMethods = [...]reflect.Method{
		reflect.TypeFor[http.Flusher]().Method(0),
		reflect.TypeFor[http.Hijacker]().Method(0),
		reflect.TypeFor[io.ReaderFrom]().Method(0),
		reflect.TypeFor[http.Pusher]().Method(0),
		//lint:ignore SA1019 plugins (and their hosts) may still use CloseNotify: yaegi v2 forwards it
		reflect.TypeFor[http.CloseNotifier]().Method(0),
		reflect.TypeFor[interface{ Unwrap() http.ResponseWriter }]().Method(0),
		reflect.TypeFor[interface{ FlushError() error }]().Method(0),
		reflect.TypeFor[interface{ SetReadDeadline(time.Time) error }]().Method(0),
		reflect.TypeFor[interface{ SetWriteDeadline(time.Time) error }]().Method(0),
		reflect.TypeFor[interface{ EnableFullDuplex() error }]().Method(0),
	}
	responseWriterType = reflect.TypeFor[http.ResponseWriter]()
)

// rwFeat is the set of optional methods of an interpreted type.
type rwFeat struct {
	mask uint16 // rwFlush...
}

// rwFeatures returns the optional ResponseWriter methods of rt, with the
// signatures of Go (a Flush() error method does not make a Flusher).
func (m *Machine) rwFeatures(rt *RType) rwFeat {
	if f, ok := m.rwCache.Load(rt); ok {
		return f.(rwFeat)
	}
	var f rwFeat
	for i, meth := range rwFeatMethods {
		if m.methodType(rt, meth.Name) == meth.Type {
			f.mask |= 1 << i
		}
	}
	m.rwCache.Store(rt, f)
	return f
}

// rwWrapper boxes the interpreted value iv, passed to native code as an
// http.ResponseWriter, in the wrapper with the optional methods of its
// dynamic type. w is the generic wrapper that ifaceWrapper chose: it
// checked Header, Write and WriteHeader. ok is false when the type has none
// of the optional methods: the generic wrapper is then used, as before.
func (m *Machine) rwWrapper(iv *Iface, w *yaegiWrapper, rt reflect.Type) (reflect.Value, bool) {
	if rt != responseWriterType || w.t != rwGenericWrapper() {
		return reflect.Value{}, false
	}
	f := m.rwFeatures(iv.T)
	if f.mask == 0 {
		return reflect.Value{}, false
	}
	return reflect.ValueOf(rwMake(rwBase{o: vmObj{m: m, iv: iv}, f: f})), true
}

// rwGenericWrapper returns the generic wrapper type of http.ResponseWriter.
func rwGenericWrapper() reflect.Type {
	wrappersOnce.Do(loadWrappers)
	return rwGeneric
}

// rwMake returns the combination wrapper for the features of b.
func rwMake(b rwBase) http.ResponseWriter {
	return rwCombos[rwComboMask(b.f.mask)](b)
}

// rwComboMask is the wrapper type used for the feature mask: the mask
// itself, or, for a type with ReadFrom, Push or CloseNotify and only some
// control methods, the mask with the four control methods (rwcombos_gen.go does not
// make these combinations).
func rwComboMask(mask uint16) uint16 {
	if mask&rwRare != 0 && mask&rwCtlAll != 0 {
		mask |= rwCtlAll
	}
	return mask
}

// rwOut is result i of an interpreted method, as a T. It converts the
// result when its type is distinct but convertible (interpreted named
// types), like the generic wrappers do.
func rwOut[T any](out []reflect.Value, i int) T {
	if i < len(out) && out[i].IsValid() {
		if t := reflect.TypeFor[T](); out[i].Type() != t && t.Kind() != reflect.Interface && out[i].Type().ConvertibleTo(t) {
			out[i] = out[i].Convert(t)
		}
	}
	return Out[T](out, i)
}

// rwBase is the part of every combination wrapper: the ResponseWriter
// methods, and the interpreted value, which comes back unchanged when native
// code gives the wrapper back to the plugin (vmBoxed). It does not embed
// vmObj, so that the wrapper has no other methods (Format, MarshalJSON).
type rwBase struct {
	o vmObj
	f rwFeat
}

func (b rwBase) iface() *Iface { return b.o.iv }
func (b rwBase) obj() vmObj    { return b.o }

func (b rwBase) Header() http.Header { return rwOut[http.Header](b.o.call("Header"), 0) }

func (b rwBase) Write(p []byte) (int, error) {
	out := b.o.call("Write", reflect.ValueOf(p))
	return rwOut[int](out, 0), rwOut[error](out, 1)
}

func (b rwBase) WriteHeader(statusCode int) { b.o.call("WriteHeader", reflect.ValueOf(statusCode)) }

type rwFlushW struct{ o vmObj }

func (w rwFlushW) Flush() { w.o.call("Flush") }

type rwHijackW struct{ o vmObj }

func (w rwHijackW) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	out := w.o.call("Hijack")
	return rwOut[net.Conn](out, 0), rwOut[*bufio.ReadWriter](out, 1), rwOut[error](out, 2)
}

type rwReadFromW struct{ o vmObj }

func (w rwReadFromW) ReadFrom(r io.Reader) (int64, error) {
	out := w.o.call("ReadFrom", reflect.ValueOf(&r).Elem())
	return rwOut[int64](out, 0), rwOut[error](out, 1)
}

type rwPushW struct{ o vmObj }

func (w rwPushW) Push(target string, opts *http.PushOptions) error {
	return rwOut[error](w.o.call("Push", reflect.ValueOf(target), reflect.ValueOf(opts)), 0)
}

// rwCloseNotifyW gives the channel of the plugin's CloseNotify. A channel
// that the plugin gets from native code (the CloseNotify of the writer
// below) goes back unchanged. A channel made by the plugin cannot be passed
// to native code (a limit of the VM for every channel, see toNativeW).
type rwCloseNotifyW struct{ o vmObj }

func (w rwCloseNotifyW) CloseNotify() <-chan bool {
	return rwOut[<-chan bool](w.o.call("CloseNotify"), 0)
}

type rwUnwrapW struct{ o vmObj }

func (w rwUnwrapW) Unwrap() http.ResponseWriter {
	return rwOut[http.ResponseWriter](w.o.call("Unwrap"), 0)
}

// The control methods. Their struct holds the whole rwBase: when the
// type does not have the method (only possible with rwComboMask), the method
// runs http.ResponseController on the same value without the control
// methods, which then uses Flush, Unwrap... as Go would.

// rwCtlRest is http.ResponseController on b without the control methods.
func rwCtlRest(b rwBase) *http.ResponseController {
	b.f.mask &^= rwCtlAll
	return http.NewResponseController(rwMake(b))
}

type rwFlushErrorW struct{ b rwBase }

func (w rwFlushErrorW) FlushError() error {
	if w.b.f.mask&rwFlushError == 0 {
		return rwCtlRest(w.b).Flush()
	}
	return rwOut[error](w.b.o.call("FlushError"), 0)
}

type rwSetReadDeadlineW struct{ b rwBase }

func (w rwSetReadDeadlineW) SetReadDeadline(t time.Time) error {
	if w.b.f.mask&rwSetReadDeadline == 0 {
		return rwCtlRest(w.b).SetReadDeadline(t)
	}
	return rwOut[error](w.b.o.call("SetReadDeadline", reflect.ValueOf(t)), 0)
}

type rwSetWriteDeadlineW struct{ b rwBase }

func (w rwSetWriteDeadlineW) SetWriteDeadline(t time.Time) error {
	if w.b.f.mask&rwSetWriteDeadline == 0 {
		return rwCtlRest(w.b).SetWriteDeadline(t)
	}
	return rwOut[error](w.b.o.call("SetWriteDeadline", reflect.ValueOf(t)), 0)
}

type rwEnableFullDuplexW struct{ b rwBase }

func (w rwEnableFullDuplexW) EnableFullDuplex() error {
	if w.b.f.mask&rwEnableFullDuplex == 0 {
		return rwCtlRest(w.b).EnableFullDuplex()
	}
	return rwOut[error](w.b.o.call("EnableFullDuplex"), 0)
}
