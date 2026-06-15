package stdlib

import (
	"errors"
	"io"
	"log"
	"os"
	"strconv"
)

var errRestricted = errors.New("restricted")

// osExit invokes panic instead of exit.
func osExit(code int) { panic("os.Exit(" + strconv.Itoa(code) + ")") }

// osFindProcess returns os.FindProcess, except for self process.
func osFindProcess(pid int) (*os.Process, error) {
	if pid == os.Getpid() {
		return nil, errRestricted
	}
	return os.FindProcess(pid)
}

// The following functions call Panic instead of Fatal to avoid exit.
func logFatal(v ...any)            { log.Panic(v...) }
func logFatalf(f string, v ...any) { log.Panicf(f, v...) }
func logFatalln(v ...any)          { log.Panicln(v...) }

type logLogger struct {
	l *log.Logger
}

// logNew Returns a wrapped logger.
func logNew(out io.Writer, prefix string, flag int) *logLogger {
	return &logLogger{log.New(out, prefix, flag)}
}

// The following methods call Panic instead of Fatal to avoid exit.
func (l *logLogger) Fatal(v ...any)            { l.l.Panic(v...) }
func (l *logLogger) Fatalf(f string, v ...any) { l.l.Panicf(f, v...) }
func (l *logLogger) Fatalln(v ...any)          { l.l.Panicln(v...) }

// The following methods just forward to wrapped logger.
func (l *logLogger) Flags() int                        { return l.l.Flags() }
func (l *logLogger) Output(d int, s string) error      { return l.l.Output(d, s) }
func (l *logLogger) Panic(v ...any)            { l.l.Panic(v...) }
func (l *logLogger) Panicf(f string, v ...any) { l.l.Panicf(f, v...) }
func (l *logLogger) Panicln(v ...any)          { l.l.Panicln(v...) }
func (l *logLogger) Prefix() string                    { return l.l.Prefix() }
func (l *logLogger) Print(v ...any)            { l.l.Print(v...) }
func (l *logLogger) Printf(f string, v ...any) { l.l.Printf(f, v...) }
func (l *logLogger) Println(v ...any)          { l.l.Println(v...) }
func (l *logLogger) SetFlags(flag int)                 { l.l.SetFlags(flag) }
func (l *logLogger) SetOutput(w io.Writer)             { l.l.SetOutput(w) }
func (l *logLogger) Writer() io.Writer                 { return l.l.Writer() }
