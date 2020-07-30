/*
Yaegi interprets Go programs.

Yaegi reads Go language programs from standard input, string
parameters or files and run them.

If invoked with no arguments, it processes the standard input in
a Read-Eval-Print-Loop. A prompt is displayed if standard input is
a terminal.

File Mode

In file mode, as in a standard Go compiler, source files are read entirely
before being parsed, then evaluated. It allows to handle forward
declarations and to have package code split in multiple source files.

Go specifications fully apply in this mode.

All files are interpreted in file mode except the initial file if it
starts with "#!" characters (the shebang pattern to allow executable
scripts), for example "#!/usr/bin/env yaegi". In that case, the initial
file is interpreted in REPL mode.

REPL mode

In REPL mode, the interpreter parses the code incrementally. As soon
as a statement is complete, it evaluates it. This makes the interpreter
suitable for interactive command line and scripts.

Go specifications apply with the following differences:

All local and global declarations (const, var, type, func) are allowed,
including in short form, except that all identifiers must be defined
before use (as declarations inside a standard Go function).

The statements are evaluated in the global space, within an implicit
"main" package.

It is not necessary to have a package statement, or a main function in
REPL mode. Import statements for preloaded binary packages can also
be avoided (i.e. all the standard library except the few packages
where default names collide, as "math/rand" and "crypto/rand", for which
an explicit import is still necessary).

Note that the source packages are always interpreted in file mode,
even if imported from REPL.

The following extract is a valid executable script:

	#!/usr/bin/env yaegi
	helloHandler := func(w http.ResponseWriter, req *http.Request) {
	   io.WriteString(w, "Hello, world!\n")
	}
	http.HandleFunc("/hello", helloHandler)
	log.Fatal(http.ListenAndServe(":8080", nil))

Example of a one liner:

	$ yaegi -e 'println(reflect.TypeOf(fmt.Print))'

Options:
	-e string
	   evaluate the string and return.
    -i
	   start an interactive REPL after file execution.
	-syscall
	   include syscall symbols.
	-tags tag,list
	   a comma-separated list of build tags to consider satisfied during
	   the interpretation.
	-unsafe
	  include unsafe symbols.

Debugging support (may be removed at any time):
  YAEGI_AST_DOT=1
    Generate and display graphviz dot of AST with dotty(1)
  YAEGI_CFG_DOT=1
    Generate and display graphviz dot of CFG with dotty(1)
  YAEGI_DOT_CMD='dot -Tsvg -ofoo.svg'
    Defines how to process the dot code generated whenever YAEGI_AST_DOT and/or
    YAEGI_CFG_DOT is enabled. If any of YAEGI_AST_DOT or YAEGI_CFG_DOT is set,
    but YAEGI_DOT_CMD is not defined, the default is to write to a .dot file
    next to the Go source file.
*/
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
)

func main() {
	var cmd string
	var err error
	var exitCode int

	log.SetFlags(log.Lshortfile) // Ease debugging.

	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	switch cmd {
	case "extract":
		err = extractCmd(os.Args[2:])
	case "help", "-h", "--help":
		err = help(os.Args[2:])
	case "run":
		err = run(os.Args[2:])
	case "test":
		err = fmt.Errorf("test not implemented")
	default:
		// If no command is given, fallback to default "run" command.
		// This allows scripts starting with "#!/usr/bin/env yaegi",
		// as passing more than 1 argument to #! executable may be not supported
		// on all platforms.
		cmd = "run"
		err = run(os.Args[1:])
	}

	if err != nil && !errors.Is(err, flag.ErrHelp) {
		err = fmt.Errorf("%s: %w", cmd, err)
		fmt.Fprintln(os.Stderr, err)
		exitCode = 1
	}
	os.Exit(exitCode)
}
