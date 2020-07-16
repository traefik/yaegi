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
	"flag"
	"fmt"
	"go/build"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/containous/yaegi/interp"
	"github.com/containous/yaegi/stdlib"
	"github.com/containous/yaegi/stdlib/syscall"
	"github.com/containous/yaegi/stdlib/unrestricted"
	"github.com/containous/yaegi/stdlib/unsafe"
)

func main() {
	var interactive bool
	var useSyscall bool
	var useUnrestricted bool
	var useUnsafe bool
	var tags string
	var cmd string
	flag.BoolVar(&interactive, "i", false, "start an interactive REPL")
	flag.BoolVar(&useSyscall, "syscall", false, "include syscall symbols")
	flag.BoolVar(&useUnrestricted, "unrestricted", false, "include unrestricted symbols")
	flag.StringVar(&tags, "tags", "", "set a list of build tags")
	flag.BoolVar(&useUnsafe, "unsafe", false, "include usafe symbols")
	flag.StringVar(&cmd, "e", "", "set the command to be executed (instead of script or/and shell)")
	flag.Usage = func() {
		fmt.Println("Usage:", os.Args[0], "[options] [script] [args]")
		fmt.Println("Options:")
		flag.PrintDefaults()
	}
	flag.Parse()
	args := flag.Args()
	log.SetFlags(log.Lshortfile)

	i := interp.New(interp.Options{GoPath: build.Default.GOPATH, BuildTags: strings.Split(tags, ",")})
	i.Use(stdlib.Symbols)
	i.Use(interp.Symbols)
	if useSyscall {
		i.Use(syscall.Symbols)
	}
	if useUnsafe {
		i.Use(unsafe.Symbols)
	}
	if useUnrestricted {
		// Use of unrestricted symbols should always follow use of stdlib symbols, to update them.
		i.Use(unrestricted.Symbols)
	}

	if cmd != `` {
		i.REPL(strings.NewReader(cmd), os.Stderr)
	}

	if len(args) == 0 {
		if interactive || cmd == `` {
			i.REPL(os.Stdin, os.Stdout)
		}
		return
	}

	// Skip first os arg to set command line as expected by interpreted main
	os.Args = os.Args[1:]
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	b, err := ioutil.ReadFile(args[0])
	if err != nil {
		log.Fatal("Could not read file: ", args[0])
	}

	if s := string(b); strings.HasPrefix(s, "#!") {
		// Allow executable go scripts, Have the same behavior as in interactive mode.
		s = strings.Replace(s, "#!", "//", 1)
		i.REPL(strings.NewReader(s), os.Stdout)
	} else {
		// Files not starting with "#!" are supposed to be pure Go, directly Evaled.
		_, err := i.Eval(s, args[0], false)
		if err != nil {
			fmt.Println(err)
			if p, ok := err.(interp.Panic); ok {
				fmt.Println(string(p.Stack))
			}
		}
	}

	if interactive {
		i.REPL(os.Stdin, os.Stdout)
	}
}
