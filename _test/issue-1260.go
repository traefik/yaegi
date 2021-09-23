package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
)

type WriteSyncer interface {
	io.Writer
	Sync() error
}

type Sink interface {
	WriteSyncer
	io.Closer
}

func newFileSink(path string) (Sink, error) {
	return os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0666)
}

type Sink1 struct{ name string }

func (s *Sink1) Write(b []byte) (int, error) { println("in Write"); return 0, nil }
func (s *Sink1) Sync() error                 { println("in Sync"); return nil }
func (s *Sink1) Close() error                { println("in Close", s.name); return nil }
func newS1(name string) Sink                 { return &Sink1{name} }

func main() {
	tmpfile, err := ioutil.TempFile("", "xxx")
	if err != nil {
		panic(err)
	}
	defer os.Remove(tmpfile.Name())
	closers := []io.Closer{}
	sink, err := newFileSink(tmpfile.Name())
	if err != nil {
		panic(err)
	}
	closers = append(closers, sink)

	s1 := newS1("test")
	closers = append(closers, s1)
	for _, closer := range closers {
		fmt.Println(closer.Close())
	}
}

// Output:
// <nil>
// in Close test
// <nil>
