// Package rwplug is a plugin that wraps the ResponseWriter before it calls
// the next handler, like logging or compression plugins do. Config.Mode
// picks the wrapper, each with a different set of optional methods.
package rwplug

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"time"
)

type Config struct {
	Mode string
}

func CreateConfig() *Config { return &Config{} }

type mw struct {
	next http.Handler
	mode string
}

func New(ctx context.Context, next http.Handler, cfg *Config, name string) (http.Handler, error) {
	return &mw{next: next, mode: cfg.Mode}, nil
}

func (m *mw) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	switch m.mode {
	case "full":
		m.next.ServeHTTP(&fullRW{ResponseWriter: w}, req)
	case "unwrap":
		m.next.ServeHTTP(&unwrapRW{w}, req)
	case "badflush":
		m.next.ServeHTTP(&badFlushRW{w}, req)
	case "deadline":
		m.next.ServeHTTP(&deadlineRW{w}, req)
	case "readfrom":
		m.next.ServeHTTP(&readFromRW{w}, req)
	case "push":
		m.next.ServeHTTP(&pushRW{w}, req)
	case "closenotify":
		m.next.ServeHTTP(&closeNotifyRW{w}, req)
	case "flusherror":
		m.next.ServeHTTP(&flushErrorRW{w}, req)
	case "readdeadline":
		m.next.ServeHTTP(&readDeadlineRW{w}, req)
	case "fullduplex":
		m.next.ServeHTTP(&fullDuplexRW{w}, req)
	case "valflush":
		// A value: its method set has no Flush, so it is not a Flusher.
		m.next.ServeHTTP(valFlushRW{w}, req)
	case "ptrflush":
		m.next.ServeHTTP(&valFlushRW{w}, req)
	case "rarectl":
		m.next.ServeHTTP(&rareCtlRW{w}, req)
	case "rarectlunwrap":
		m.next.ServeHTTP(&rareCtlUnwrapRW{w}, req)
	case "flushpanic":
		m.next.ServeHTTP(&flushPanicRW{w}, req)
	case "flushloop":
		m.next.ServeHTTP(&flushLoopRW{ResponseWriter: w}, req)
	default:
		m.next.ServeHTTP(&plainRW{ResponseWriter: w}, req)
	}
}

// fullRW has Flush, Hijack and Unwrap, and counts the bytes written.
type fullRW struct {
	http.ResponseWriter
	n int
}

func (r *fullRW) Write(p []byte) (int, error) {
	n, err := r.ResponseWriter.Write(p)
	r.n += n
	return n, err
}

func (r *fullRW) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (r *fullRW) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return http.NewResponseController(r.ResponseWriter).Hijack()
}

func (r *fullRW) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// plainRW only overrides Write: in Go it is not a Flusher or a Hijacker
// (the embedded interface only gives the methods of ResponseWriter).
type plainRW struct {
	http.ResponseWriter
	n int
}

func (r *plainRW) Write(p []byte) (int, error) {
	n, err := r.ResponseWriter.Write(p)
	r.n += n
	return n, err
}

// unwrapRW only has Unwrap: http.ResponseController reaches the writer below.
type unwrapRW struct{ http.ResponseWriter }

func (r *unwrapRW) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// badFlushRW has a Flush method with another signature: not a Flusher.
type badFlushRW struct{ http.ResponseWriter }

func (r *badFlushRW) Flush() error { return nil }

// deadlineRW has SetWriteDeadline, without Unwrap.
type deadlineRW struct{ http.ResponseWriter }

func (r *deadlineRW) SetWriteDeadline(t time.Time) error {
	r.Header().Set("X-Deadline", "set")
	return nil
}

// readFromRW has ReadFrom only.
type readFromRW struct{ http.ResponseWriter }

func (r *readFromRW) ReadFrom(src io.Reader) (int64, error) {
	r.Header().Set("X-Via", "readfrom")
	return io.Copy(r.ResponseWriter, src)
}

// pushRW has Push only.
type pushRW struct{ http.ResponseWriter }

func (r *pushRW) Push(target string, opts *http.PushOptions) error {
	if opts == nil {
		return errors.New("pushed " + target)
	}
	return errors.New("pushed " + target + " " + opts.Method)
}

// closeNotifyRW has CloseNotify only. It gives the channel of the writer
// below.
type closeNotifyRW struct{ http.ResponseWriter }

func (r *closeNotifyRW) CloseNotify() <-chan bool {
	r.Header().Set("X-Close-Notify", "plugin")
	return r.ResponseWriter.(http.CloseNotifier).CloseNotify()
}

// flushErrorRW has FlushError only: http.ResponseController uses it, but it
// is not a Flusher.
type flushErrorRW struct{ http.ResponseWriter }

func (r *flushErrorRW) FlushError() error { return errors.New("plugin flusherror") }

// readDeadlineRW has SetReadDeadline only.
type readDeadlineRW struct{ http.ResponseWriter }

func (r *readDeadlineRW) SetReadDeadline(t time.Time) error {
	r.Header().Set("X-Read-Deadline", "set")
	return nil
}

// fullDuplexRW has EnableFullDuplex only.
type fullDuplexRW struct{ http.ResponseWriter }

func (r *fullDuplexRW) EnableFullDuplex() error { return errors.New("plugin fullduplex") }

// valFlushRW has Flush with a pointer receiver: valFlushRW is not a Flusher,
// *valFlushRW is one.
type valFlushRW struct{ http.ResponseWriter }

func (r *valFlushRW) Flush() { r.Header().Set("X-Flush", "plugin") }

// rareCtlRW has ReadFrom and FlushError only: the wrapper of the VM has the
// four control methods (rwComboMask), and the three that the type does not
// have must give the results of Go through http.ResponseController.
type rareCtlRW struct{ http.ResponseWriter }

func (r *rareCtlRW) ReadFrom(src io.Reader) (int64, error) {
	r.Header().Set("X-Via", "readfrom")
	return io.Copy(r.ResponseWriter, src)
}

func (r *rareCtlRW) FlushError() error { return errors.New("plugin flusherror") }

// rareCtlUnwrapRW is rareCtlRW with Unwrap: http.ResponseController reaches
// the writer below for the control methods that the type does not have.
type rareCtlUnwrapRW struct{ http.ResponseWriter }

func (r *rareCtlUnwrapRW) ReadFrom(src io.Reader) (int64, error) {
	r.Header().Set("X-Via", "readfrom")
	return io.Copy(r.ResponseWriter, src)
}

func (r *rareCtlUnwrapRW) FlushError() error { return errors.New("plugin flusherror") }

func (r *rareCtlUnwrapRW) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// flushPanicRW has a Flush that panics.
type flushPanicRW struct{ http.ResponseWriter }

func (r *flushPanicRW) Flush() { panic("flush boom") }

// flushLoopRW has a Flush that never ends (it stops with the budget).
type flushLoopRW struct {
	http.ResponseWriter
	n int
}

func (r *flushLoopRW) Flush() {
	for {
		r.n++
	}
}
