package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/traefik/yaegi/v2/vm"
)

// evalOptions returns the options of a session for the flags of yaegi: the
// standard library of a program run with "go run", unless restricted.
func evalOptions(budget int64, restricted, useUnsafe, aot, staleAOT, printStdout bool) vm.Options {
	opts := vm.Options{
		Unrestricted:  !restricted,
		RealProcess:   !restricted,
		UseUnsafe:     useUnsafe,
		Budget:        budget,
		AOT:           aot,
		AllowStaleAOT: staleAOT,
	}
	if printStdout {
		opts.PrintOut = os.Stdout
	}
	return opts
}

// runEval runs src (-e), or the inputs read from the standard input (-i),
// in one session, and returns the exit code.
func runEval(opts vm.Options, src string, interactive bool) int {
	s, err := vm.NewSession(opts)
	if err != nil {
		return exitCode(err)
	}
	ctx := context.Background()
	if !interactive {
		if err := evalPrint(ctx, s, src); err != nil {
			if errors.Is(err, vm.ErrIncomplete) {
				fmt.Fprintln(os.Stderr, "yaegi: -e: incomplete input")
				return 4
			}
			return exitCode(err)
		}
		return 0
	}
	in := bufio.NewReader(os.Stdin)
	var buf strings.Builder
	for {
		if buf.Len() == 0 {
			fmt.Fprint(os.Stderr, "> ")
		} else {
			fmt.Fprint(os.Stderr, ". ")
		}
		line, rerr := in.ReadString('\n')
		buf.WriteString(line)
		if strings.TrimSpace(buf.String()) == "" {
			buf.Reset()
			if rerr != nil {
				fmt.Fprintln(os.Stderr)
				return 0
			}
			continue
		}
		err := evalPrint(ctx, s, buf.String())
		if errors.Is(err, vm.ErrIncomplete) && rerr == nil {
			continue // read the next line of the input
		}
		buf.Reset()
		if err != nil {
			exitCode(err) // prints the error
		}
		if rerr != nil {
			fmt.Fprintln(os.Stderr)
			return 0
		}
	}
}

// evalPrint runs src in s and prints its values, as fmt.Println does in the
// session (with the String methods of its types).
func evalPrint(ctx context.Context, s *vm.Session, src string) error {
	res, err := s.Eval(ctx, src)
	if err != nil || len(res) == 0 {
		return err
	}
	str, err := s.Sprint(ctx)
	if err != nil {
		return err
	}
	fmt.Print(str)
	return nil
}
