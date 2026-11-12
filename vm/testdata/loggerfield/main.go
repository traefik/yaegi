// Command loggerfield stores loggers of package log in native fields of type
// *log.Logger (httputil.ReverseProxy.ErrorLog, http.Server.ErrorLog), as
// plugins do, and reads them back: the logger writes to its output and is
// equal to the one stored. It panics on the first difference.
// TestLoggerField runs it with RestrictedStd and with Restricted, where
// log.Logger is a wrapper type; it also passes with go run.
package main

import (
	"bytes"
	"log"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"reflect"
	"runtime"
)

type config struct {
	Log *log.Logger
}

func main() {
	var buf bytes.Buffer
	l := log.New(&buf, "proxy: ", 0)
	rp := &httputil.ReverseProxy{ErrorLog: l}
	rp.ErrorLog.Println("hello")
	if got := buf.String(); got != "proxy: hello\n" {
		panic("ErrorLog wrote " + got)
	}
	if rp.ErrorLog != l {
		panic("ReverseProxy.ErrorLog is not the logger stored")
	}

	srv := http.Server{ErrorLog: log.Default()}
	if srv.ErrorLog != log.Default() {
		panic("Server.ErrorLog is not log.Default()")
	}
	srv.ErrorLog = nil
	if srv.ErrorLog != nil {
		panic("Server.ErrorLog is not nil")
	}

	var proxies []httputil.ReverseProxy
	proxies = append(proxies, httputil.ReverseProxy{ErrorLog: l})
	if proxies[0].ErrorLog != l {
		panic("ErrorLog of a slice element is not the logger stored")
	}

	// The logger comes from a field of a struct of the program, a map value
	// and an interface.
	c := config{Log: log.New(&buf, "config: ", 0)}
	srv.ErrorLog = c.Log
	m := map[string]*log.Logger{"a": c.Log}
	rp.ErrorLog = m["a"]
	var x any = rp.ErrorLog
	if y, ok := x.(*log.Logger); !ok || y != c.Log || srv.ErrorLog != c.Log || rp.ErrorLog != m["a"] {
		panic("the logger of a struct field, a map value or an interface is not the logger stored")
	}
	loggers := []*log.Logger{rp.ErrorLog, nil}
	if loggers[0] != c.Log || loggers[1] != nil {
		panic("a slice of loggers read from native fields is wrong")
	}

	// A native logger that is no wrapper (slog.NewLogLogger): stored and
	// read back as is.
	n := slog.NewLogLogger(slog.NewTextHandler(&buf, nil), slog.LevelInfo)
	srv.ErrorLog = n
	if srv.ErrorLog != n {
		panic("Server.ErrorLog is not the slog logger stored")
	}

	// Stored with reflect (Value.Set), and set to nil the same way.
	f := reflect.ValueOf(rp).Elem().FieldByName("ErrorLog")
	f.Set(reflect.ValueOf(c.Log))
	if rp.ErrorLog != c.Log {
		panic("ErrorLog set with reflect is not the logger stored")
	}
	f.Set(reflect.Zero(f.Type()))
	if rp.ErrorLog != nil {
		panic("ErrorLog set to zero with reflect is not nil")
	}

	// Only the native field keeps the logger: the program drops its value,
	// and the garbage collector may take the wrapper. The field still
	// writes, in a loop (the fast call path), and two reads are equal.
	buf.Reset()
	rp.ErrorLog = log.New(&buf, "gone: ", 0)
	runtime.GC()
	runtime.GC()
	for range 3 {
		rp.ErrorLog.Print("x")
	}
	if got := buf.String(); got != "gone: x\ngone: x\ngone: x\n" {
		panic("ErrorLog without its program value wrote " + got)
	}
	if a, b := rp.ErrorLog, rp.ErrorLog; a != b {
		panic("two reads of ErrorLog differ")
	}
}
