package main

import (
	"log"
	"os"
)

type Logger struct {
	m []*log.Logger
}

func (l *Logger) Infof(format string, args ...any) {
	l.m[0].Printf(format, args...)
}

func main() {
	l := &Logger{m: []*log.Logger{log.New(os.Stdout, "", log.Lmsgprefix)}}
	l.Infof("test %s", "test")
}

// Output:
// test test
