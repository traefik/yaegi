package main

import (
	"fmt"
	"go/scanner"
	"io"
	"os"
	"strings"

	"github.com/peterh/liner"

	"github.com/containous/yaegi/interp"
)

var (
	history []string
)

func repl(interp *interp.Interpreter, in *os.File, out *os.File) {
	line := liner.NewLiner()
	defer line.Close()

	line.SetCtrlCAborts(true)

	line.SetCompleter(func(line string) (c []string) {
		for _, n := range history {
			if strings.HasPrefix(n, strings.ToLower(line)) {
				c = append(c, n)
			}
		}
		return
	})

	src := ""
	for {
		if srcLine, err := line.Prompt(getPrompt(in, out)); err == nil {
			line.AppendHistory(srcLine)
			src += srcLine + "\n"
			if v, err := interp.Eval(src); err != nil {
				switch err.(type) {
				case scanner.ErrorList:
					// Early failure in the scanner: the source is incomplete
					// and no AST could be produced, neither compiled / run.
					// Get one more line, and retry
					continue
				default:
					fmt.Fprintln(out, err)
				}
			} else if v.IsValid() {
				fmt.Fprintln(out, v)
			}
			src = ""
		} else if err == liner.ErrPromptAborted {
			fmt.Fprintf(out, "Aborted")
			return
		} else if err == io.EOF {
			return
		} else {
			fmt.Fprintf(out, "Error reading line: ", err)
			return
		}
	}
}

func getPrompt(in, out *os.File) string {
	if stat, err := in.Stat(); err == nil && stat.Mode()&os.ModeCharDevice != 0 {
		return "> "
	}
	return ""
}
