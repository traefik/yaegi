package main

import (
	"flag"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"

	"github.com/traefik/yaegi/internal/dap"
	"github.com/traefik/yaegi/internal/dbg"
	"github.com/traefik/yaegi/internal/iox"
)

var mode = flag.String("mode", "stdio", "Listening mode, stdio|net")
var addr = flag.String("addr", "tcp://localhost:16348", "Net address to listen on, must be a TCP or Unix socket URL")
var logFile = flag.String("log", "", "Log protocol messages to a file")
var gopath = flag.String("gopath", "", "GOPATH")
var stopAtEntry = flag.Bool("stop-at-entry", false, "Stop at program entry")

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func main() {
	flag.Parse()

	if flag.NArg() < 1 {
		fatalf("Usage: %s [options] <program.go>", os.Args[0])
	}

	var l net.Listener
	switch *mode {
	case "stdio":
		l = iox.NewStdio()

	case "net":
		u, err := url.Parse(*addr)
		if err != nil {
			fatalf("%v", err)
		}

		var addr string
		if u.Scheme == "unix" {
			addr = u.Path
			if _, err := os.Stat(addr); err == nil {
				os.Remove(addr)
			}
			defer os.Remove(addr)
		} else {
			addr = u.Host
		}
		l, err = net.Listen(u.Scheme, addr)
		if err != nil {
			fatalf("%v", err)
		}

	default:
		fatalf("Invalid mode %q", *mode)
	}

	st, err := os.Stat(flag.Arg(0))
	if err != nil {
		fatalf("source: %v", err)
	} else if st.IsDir() {
		fatalf("source is dir: %q", flag.Arg(0))
	}

	opts := dbg.Options{
		GoPath:      *gopath,
		StopAtEntry: *stopAtEntry,
	}
	adp := dbg.NewEvalPathAdapter(flag.Arg(0), &opts)
	srv := dap.NewServer(l, adp)

	var lf io.Writer
	if *logFile == "-" {
		lf = os.Stderr
	} else if *logFile != "" {
		f, err := os.Create(*logFile)
		if err != nil {
			fatalf("log: %v", err)
		}
		defer f.Close()
		lf = f
	}

	s, err := srv.Accept()
	if err != nil {
		fatalf("%v", err)
	}

	s.Debug(lf)
	s.Run()
}
