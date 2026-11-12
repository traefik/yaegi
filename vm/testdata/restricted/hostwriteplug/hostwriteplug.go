// Package hostwriteplug writes to variables of the standard library, which
// are the host's, and calls functions that change the signal handling of
// the process. None of this may load.
package hostwriteplug

import (
	"bytes"
	"crypto/tls"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/rpc"
	"os"
	"os/signal"
	"time"
	"unicode"
)

func Write() {
	time.Local = time.FixedZone("EVIL", 3600)
	*time.UTC = *time.FixedZone("EVIL", 3600)
	io.EOF = errors.New("x")
	unicode.Categories["Lu"] = nil
	delete(unicode.Scripts, "Latin")
	p := &io.ErrUnexpectedEOF
	_ = p
	http.ErrAbortHandler, http.ErrBodyNotAllowed = nil, nil
	tls.CipherSuites()[0].Name = "allowed: not a variable"
	*os.Stdout = *os.Stderr // the plugin's copy points to the host's *os.File
	signal.Ignore(os.Interrupt)
	signal.Reset()
}

// Allowed writes and calls: the copies of the plugin (stdio.go and
// hostregs.go), and local variables. rpc.HandleHTTP registers on the
// plugin's own mux (hostregs.go).
func Allowed() {
	http.DefaultClient = &http.Client{}
	http.DefaultTransport.(*http.Transport).MaxIdleConns = 1
	*http.DefaultServeMux = http.ServeMux{}
	os.Args[0] = "plugin"
	os.Stdout = os.Stderr
	loc := time.Local
	loc = nil
	_ = loc
	os.Stdout.Write(nil)
	signal.Notify(make(chan os.Signal, 1), os.Interrupt)
	slog.SetDefault(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	slog.SetLogLoggerLevel(slog.LevelDebug)
	rpc.DefaultServer = rpc.NewServer()
	rpc.HandleHTTP()
	rpc.NewServer().HandleHTTP("/rpc", "/debug/rpc")
}
