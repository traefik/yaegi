package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"go/build"
	"io"
	"io/ioutil"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/traefik/yaegi/internal/dap"
	"github.com/traefik/yaegi/internal/dbg"
	"github.com/traefik/yaegi/internal/iox"
	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
	"github.com/traefik/yaegi/stdlib/syscall"
	"github.com/traefik/yaegi/stdlib/unrestricted"
	"github.com/traefik/yaegi/stdlib/unsafe"
)

func debugServer(arg []string) error {
	var (
		mode          string
		addr          string
		logFile       string
		stopAtEntry   bool
		singleSession bool
		tags          string
		noAutoImport  bool
		err           error
	)

	// The following flags are initialized from environment.
	useSyscall, _ := strconv.ParseBool(os.Getenv("YAEGI_SYSCALL"))
	useUnrestricted, _ := strconv.ParseBool(os.Getenv("YAEGI_UNRESTRICTED"))
	useUnsafe, _ := strconv.ParseBool(os.Getenv("YAEGI_UNSAFE"))

	dflag := flag.NewFlagSet("debug", flag.ContinueOnError)
	dflag.StringVar(&mode, "mode", "stdio", "Listening mode, stdio|net")
	dflag.StringVar(&addr, "addr", "tcp://localhost:16348", "Net address to listen on, must be a TCP or Unix socket URL")
	dflag.StringVar(&logFile, "log", "", "Log protocol messages to a file")
	dflag.BoolVar(&stopAtEntry, "stop-at-entry", false, "Stop at program entry")
	dflag.BoolVar(&singleSession, "single-session", true, "Run a single debug session and exit once it terminates")
	dflag.StringVar(&tags, "tags", "", "set a list of build tags")
	dflag.BoolVar(&useSyscall, "syscall", useSyscall, "include syscall symbols")
	dflag.BoolVar(&useUnrestricted, "unrestricted", useUnrestricted, "include unrestricted symbols")
	dflag.BoolVar(&useUnsafe, "unsafe", useUnsafe, "include unsafe symbols")
	dflag.BoolVar(&noAutoImport, "noautoimport", false, "do not auto import pre-compiled packages. Import names that would result in collisions (e.g. rand from crypto/rand and rand from math/rand) are automatically renamed (crypto_rand and math_rand)")
	dflag.Usage = func() {
		fmt.Println("Usage: yaegi debug [options] <path> [args]")
		fmt.Println("Options:")
		dflag.PrintDefaults()
	}
	if err = dflag.Parse(arg); err != nil {
		return err
	}
	args := dflag.Args()

	if len(args) == 0 {
		return errors.New("missing script path")
	}

	newInterp := func(opts interp.Options) (*interp.Interpreter, error) {
		opts.GoPath = build.Default.GOPATH
		opts.BuildTags = strings.Split(tags, ",")
		i := interp.New(opts)
		if err := i.Use(stdlib.Symbols); err != nil {
			return nil, err
		}
		if err := i.Use(interp.Symbols); err != nil {
			return nil, err
		}
		if useSyscall {
			if err := i.Use(syscall.Symbols); err != nil {
				return nil, err
			}
			// Using a environment var allows a nested interpreter to import the syscall package.
			if err := os.Setenv("YAEGI_SYSCALL", "1"); err != nil {
				return nil, err
			}
		}
		if useUnsafe {
			if err := i.Use(unsafe.Symbols); err != nil {
				return nil, err
			}
			if err := os.Setenv("YAEGI_UNSAFE", "1"); err != nil {
				return nil, err
			}
		}
		if useUnrestricted {
			// Use of unrestricted symbols should always follow stdlib and syscall symbols, to update them.
			if err := i.Use(unrestricted.Symbols); err != nil {
				return nil, err
			}
			if err := os.Setenv("YAEGI_UNRESTRICTED", "1"); err != nil {
				return nil, err
			}
		}

		return i, nil
	}

	errch := make(chan error)
	go func() {
		for err := range errch {
			fmt.Printf("ERR %v\n", err)
		}
	}()
	defer close(errch)

	opts := &dbg.Options{
		StopAtEntry:    stopAtEntry,
		NewInterpreter: newInterp,
		Errors:         errch,
	}

	var adp *dbg.Adapter
	if src, ok := isScript(args[0]); ok {
		opts.NewInterpreter = func(opts interp.Options) (*interp.Interpreter, error) {
			i, err := newInterp(opts)
			if err != nil {
				return nil, err
			}

			if !noAutoImport {
				i.ImportUsed()
			}
			return i, nil
		}

		adp = dbg.NewEvalAdapter(src, opts)
	} else {
		adp = dbg.NewEvalPathAdapter(args[0], opts)
	}

	var l net.Listener
	switch mode {
	case "stdio":
		l = iox.NewStdio()

	case "net":
		u, err := url.Parse(addr)
		if err != nil {
			return err
		}

		var addr string
		if u.Scheme == "unix" {
			addr = u.Path
			if _, err := os.Stat(addr); err == nil {
				// Remove any pre-existing connection
				os.Remove(addr) //nolint:errcheck
			}

			// Remove when done
			defer os.Remove(addr) //nolint:errcheck
		} else {
			addr = u.Host
		}
		l, err = net.Listen(u.Scheme, addr)
		if err != nil {
			return err
		}

	default:
		return fmt.Errorf("Invalid mode %q", mode)
	}

	srv := dap.NewServer(l, adp)

	var lf io.Writer
	if logFile == "-" {
		lf = os.Stderr
	} else if logFile != "" {
		f, err := os.Create(logFile)
		if err != nil {
			return fmt.Errorf("log: %v", err)
		}
		defer f.Close() //nolint:errcheck
		lf = f
	}

	if singleSession {
		s, c, err := srv.Accept()
		if err != nil {
			return err
		}
		defer c.Close() //nolint:errcheck

		s.Debug(lf)
		return s.Run()
	}

	for {
		s, c, err := srv.Accept()
		if err != nil {
			return err
		}
		defer c.Close() //nolint:errcheck

		lf, addr := lf, c.RemoteAddr()
		if lf != nil {
			prefix := []byte(fmt.Sprintf("{%v}", addr))
			lf = iox.WriterFunc(func(b []byte) (int, error) {
				n, err := lf.Write(append(prefix, b...))
				if n < len(prefix) {
					n = 0
				} else {
					n -= len(prefix)
				}
				return n, err
			})
		}

		s.Debug(lf)
		err = s.Run()
		if err != nil {
			fmt.Printf("{%v} ERR %v\n", addr, err)
		}
	}
}

func isScript(path string) (src string, ok bool) {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return "", false
	}

	if !bytes.HasPrefix(b, []byte("#!")) {
		return "", false
	}

	b[0], b[1] = '/', '/'
	return string(b), true
}
