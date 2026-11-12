package stdsyms

// Replacements of the stdlib functions a plugin must not use as is (the names
// internal/extract substitutes): a plugin cannot stop the host or find it.

import (
	"errors"
	"io"
	"log"
	"os"
	"reflect"
	"strconv"
)

var errRestricted = errors.New("restricted")

// osExit panics instead of exiting.
func osExit(code int) { panic("os.Exit(" + strconv.Itoa(code) + ")") }

// osFindProcess refuses the current process.
func osFindProcess(pid int) (*os.Process, error) {
	if pid == os.Getpid() {
		return nil, errRestricted
	}
	return os.FindProcess(pid)
}

// The Fatal functions panic instead of exiting.
func logFatal(v ...any)                 { log.Panic(v...) }
func logFatalf(format string, v ...any) { log.Panicf(format, v...) }
func logFatalln(v ...any)               { log.Panicln(v...) }

// logLogger is the log.Logger of plugins (logNew returns one), whose Fatal
// methods panic. The Fatal method stays at line 39: yaegi's conformance
// test restricted3 prints the file:line of the logger (log.Lshortfile),
// that of the same method in yaegi's restricted.go.
type logLogger struct{ l *log.Logger }

func (l *logLogger) Fatal(v ...any)                 { l.l.Panic(v...) }
func (l *logLogger) Fatalf(format string, v ...any) { l.l.Panicf(format, v...) }
func (l *logLogger) Fatalln(v ...any)               { l.l.Panicln(v...) }

func (l *logLogger) Flags() int                       { return l.l.Flags() }
func (l *logLogger) Output(depth int, s string) error { return l.l.Output(depth+1, s) }
func (l *logLogger) Panic(v ...any)                   { l.l.Panic(v...) }
func (l *logLogger) Panicf(format string, v ...any)   { l.l.Panicf(format, v...) }
func (l *logLogger) Panicln(v ...any)                 { l.l.Panicln(v...) }
func (l *logLogger) Prefix() string                   { return l.l.Prefix() }
func (l *logLogger) Print(v ...any)                   { l.l.Print(v...) }
func (l *logLogger) Printf(format string, v ...any)   { l.l.Printf(format, v...) }
func (l *logLogger) Println(v ...any)                 { l.l.Println(v...) }
func (l *logLogger) SetFlags(flag int)                { l.l.SetFlags(flag) }
func (l *logLogger) SetOutput(w io.Writer)            { l.l.SetOutput(w) }
func (l *logLogger) SetPrefix(prefix string)          { l.l.SetPrefix(prefix) }
func (l *logLogger) Writer() io.Writer                { return l.l.Writer() }

func logNew(out io.Writer, prefix string, flag int) *logLogger {
	return pluginLogger(log.New(out, prefix, flag))
}

// logDefault returns the standard logger as a plugin logger, whose Fatal
// methods panic: the generated table maps log.Logger to logLogger, so the
// native *log.Logger of log.Default would exit the host. It is the same
// plugin logger while one is alive (log.Default() == log.Default()).
func logDefault() *logLogger { return pluginLogger(log.Default()) }

// Overrides of generated entries (the generated files' init functions run
// first: their names sort before this file's).
func init() {
	Symbols["log/log"]["Default"] = reflect.ValueOf(logDefault)
}
