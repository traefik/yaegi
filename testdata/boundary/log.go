// Native boundary of log: program types given to the print functions of log
// (docs/BOUNDARY.md).
package main

import (
	"fmt"
	"log"
	"strings"
)

var cur string

// probe runs one probe: its lines start with its id, and its panic is printed.
func probe(id string, f func()) {
	cur = id
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("%s: panic: %T: %v\n", id, r, r)
		}
	}()
	f()
}

// out prints one line of the current probe.
func out(format string, args ...any) { fmt.Printf(cur+": "+format+"\n", args...) }

// sv has a String method (value receiver).
type sv struct{ n int }

func (s sv) String() string { return fmt.Sprintf("sv<%d>", s.n) }

// ep is an error (pointer receiver).
type ep struct{ msg string }

func (e *ep) Error() string { return "ep:" + e.msg }

// fv has a Format method.
type fv struct{ n int }

func (f fv) Format(s fmt.State, verb rune) { fmt.Fprintf(s, "fv[%c %d]", verb, f.n) }

// gs has a GoString method.
type gs struct{ n int }

func (g gs) GoString() string { return fmt.Sprintf("gs(%d)", g.n) }

// gstr is a generic Stringer.
type gstr[T any] struct{ v T }

func (g gstr[T]) String() string { return fmt.Sprintf("gstr<%v>", g.v) }

// Types without methods.
type (
	I  int
	S  string
	Bs []byte
	pt struct{ X, Y int }
)

// lineWriter is a program writer: each Write is one line.
type lineWriter struct{ lines []string }

func (w *lineWriter) Write(p []byte) (int, error) {
	w.lines = append(w.lines, strings.TrimSuffix(string(p), "\n"))
	return len(p), nil
}

// args are the values given to every function.
var args = []any{sv{1}, &ep{"e"}, fv{2}, gs{3}, gstr[int]{4}, I(5), S("s"), Bs("b"), pt{6, 7}, []any{sv{8}, I(9)}, map[S]any{"k": fv{10}}}

const format = "%v|%s|%v|%#v|%v|%d|%q|%x|%+v|%v|%v"

func main() {
	// row: log (*Logger).Fatal,(*Logger).Fatalf,(*Logger).Fatalln,(*Logger).Output,(*Logger).Panic,(*Logger).Panicf,(*Logger).Panicln,(*Logger).Print,(*Logger).Printf,(*Logger).Println assert x.(T), T unnamed
	// row: log (*Logger).Fatal,(*Logger).Fatalf,(*Logger).Fatalln,(*Logger).Output,(*Logger).Panic,(*Logger).Panicf,(*Logger).Panicln,(*Logger).Print,(*Logger).Printf,(*Logger).Println panic x.(T) without ok, T unnamed: *runtime.TypeAssertionError
	probe("Logger", func() {
		// Fatal, Fatalf and Fatalln end the program: they format as Print,
		// Printf and Println then call Output, which is tested here.
		w := &lineWriter{}
		l := log.New(w, "pfx: ", 0)
		l.Print(args...)
		l.Printf(format, args...)
		l.Println(args...)
		l.Output(1, fmt.Sprint(sv{11}, I(12)))
		for _, f := range []func(){
			func() { l.Panic(args...) },
			func() { l.Panicf(format, args...) },
			func() { l.Panicln(args...) },
		} {
			func() {
				defer func() {
					r := recover()
					out("panic %T %q", r, r)
				}()
				f()
			}()
		}
		l.SetFlags(log.Lmsgprefix)
		l.SetPrefix("[msg] ")
		l.Println(sv{13}, &ep{"14"})
		var sb strings.Builder
		l.SetOutput(&sb)
		l.Printf("%v %v", gstr[string]{"x"}, fv{15})
		w.lines = append(w.lines, strings.TrimSuffix(sb.String(), "\n"))
		for _, line := range w.lines {
			out("%q", line)
		}
	})
	// row: log Fatal,Fatalf,Fatalln,Output,Panic,Panicf,Panicln,Print,Printf,Println assert x.(T), T unnamed
	// row: log Fatal,Fatalf,Fatalln,Output,Panic,Panicf,Panicln,Print,Printf,Println panic x.(T) without ok, T unnamed: *runtime.TypeAssertionError
	probe("Std", func() {
		// Fatal, Fatalf and Fatalln end the program: see Logger above.
		w := &lineWriter{}
		log.SetOutput(w)
		log.SetFlags(0)
		log.SetPrefix("std: ")
		log.Print(args...)
		log.Printf(format, args...)
		log.Println(args...)
		log.Output(1, fmt.Sprintln(gs{16}, pt{17, 18}))
		for _, f := range []func(){
			func() { log.Panic(args...) },
			func() { log.Panicf(format, args...) },
			func() { log.Panicln(args...) },
		} {
			func() {
				defer func() {
					r := recover()
					out("panic %T %q", r, r)
				}()
				f()
			}()
		}
		out("writer %T", log.Writer())
		for _, line := range w.lines {
			out("%q", line)
		}
	})
}
