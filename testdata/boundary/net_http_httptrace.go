// Native boundary of net/http/httptrace: program types given to httptrace (docs/BOUNDARY.md).
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptrace"
	"net/textproto"
	"strings"
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

// hook gives a GetConn hook of a generic function.
func hook[T ~string](tag T) func(string) {
	return func(hp string) { out("%s GetConn %s", tag, hp) }
}

// tag is a named string type for the generic hook.
type tag string

// recorder has hook methods (method values become hooks).
type recorder struct{ name string }

func (r *recorder) dnsStart(i httptrace.DNSStartInfo) { out("%s DNSStart %s", r.name, i.Host) }
func (r recorder) wroteHeaders()                      { out("%s WroteHeaders", r.name) }

// The server logs of net/http go to the default logger: no time in them.
func init() { log.SetFlags(0) }

func main() {
	// row: net/http/httptrace WithClientTrace reflect *
	probe("WithClientTrace/compose", func() {
		old := &httptrace.ClientTrace{
			GetConn:  hook(tag("old")),
			DNSStart: (&recorder{"old"}).dnsStart,
			Got1xxResponse: func(code int, h textproto.MIMEHeader) error {
				out("old Got1xx %d %s", code, h.Get("X"))
				return nil
			},
			GotFirstResponseByte: func() { out("old GotFirstResponseByte") },
			WroteHeaderField: func(k string, v []string) {
				out("old WroteHeaderField %s=%v", k, v)
			},
		}
		nw := &httptrace.ClientTrace{
			GetConn:      hook("new"),
			WroteHeaders: recorder{"new"}.wroteHeaders,
			Got1xxResponse: func(code int, h textproto.MIMEHeader) error {
				out("new Got1xx %d", code)
				return errors.New("new error")
			},
			PutIdleConn: func(err error) { out("new PutIdleConn %v", err) },
		}
		ctx := httptrace.WithClientTrace(context.Background(), old)
		ctx = httptrace.WithClientTrace(ctx, nw)
		tr := httptrace.ContextClientTrace(ctx)
		out("same new: %v", tr == nw)
		tr.GetConn("host:80")
		tr.DNSStart(httptrace.DNSStartInfo{Host: "dns.example"})
		err := tr.Got1xxResponse(103, textproto.MIMEHeader{"X": {"y"}})
		out("Got1xx err: %v", err)
		tr.GotFirstResponseByte()
		tr.PutIdleConn(errors.New("idle"))
		out("nil hooks: %v %v", tr.ConnectStart == nil, tr.WroteRequest == nil)
		// net/http calls the composed hooks itself.
		req, _ := http.NewRequestWithContext(ctx, "GET", "http://example.com/", nil)
		req.Header.Set("User-Agent", "probe")
		var sb strings.Builder
		out("Write: %v %d", req.Write(&sb), sb.Len())
		// A third trace over the composed one.
		ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
			GetConn: func(hp string) { out("third GetConn %s", hp) },
		})
		httptrace.ContextClientTrace(ctx).GetConn("h3:1")
	})
}
