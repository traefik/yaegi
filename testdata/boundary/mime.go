// Native boundary of mime: program types given to mime (docs/BOUNDARY.md).
package main

import (
	"fmt"
	"mime"
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

// ext and mtype are named string types of the program.
type ext string
type mtype string

// exts is a named type over []string: the []string of mime is not it.
type exts []string

// gname is a generic type that gives a string.
type gname[T any] struct{ v T }

func (g gname[T]) String() string { return fmt.Sprintf(".zg%v", g.v) }

func main() {
	// The []string values in the maps of mime are always stored by mime
	// itself: a program cannot put another type there. The probe gives
	// program types where the API takes strings and reads the lists back.
	// row: mime AddExtensionType,ExtensionsByType,TypeByExtension assert x.(T), T unnamed
	probe("ExtensionType", func() {
		e := ext(".zbq1")
		t := mtype("application/x-zbq1")
		out("Add: %v", mime.AddExtensionType(string(e), string(t)))
		out("Add 2: %v", mime.AddExtensionType(string(ext(".zbq2")), string(t)+"; charset=utf-8"))
		g := gname[int]{7}
		out("Add g: %v", mime.AddExtensionType(g.String(), "text/x-zg7"))
		out("Type: %q", mime.TypeByExtension(string(e)))
		out("Type upper: %q", mime.TypeByExtension(".ZBQ1"))
		out("Type g: %q", mime.TypeByExtension(g.String()))
		l, err := mime.ExtensionsByType(string(t))
		var x exts = l
		out("Exts: %v %v %d", l, err, len(x))
		var a any = l
		_, isNamed := a.(exts)
		_, isSlice := a.([]string)
		out("Exts type: named=%v slice=%v", isNamed, isSlice)
		l, err = mime.ExtensionsByType("text/x-zg7")
		out("Exts g: %v %v", l, err)
		l, err = mime.ExtensionsByType("application/x-zbq-none")
		out("Exts none: %v %v", l, err)
		out("Add bad: %v", mime.AddExtensionType("zbq3", "x"))
	})
	// row: mime AddExtensionType,ExtensionsByType,TypeByExtension panic x.(T) without ok, T unnamed: *runtime.TypeAssertionError
	probe("ExtensionType/panic", func() {
		// The assertion cannot fail: mime stores only []string. The probe
		// shows that no panic comes from the lookups.
		for _, s := range []string{".zbq1", ".zbq2", ".nothing-zbq"} {
			out("Type %s: %q", s, mime.TypeByExtension(s))
		}
		l, err := mime.ExtensionsByType("application/x-zbq1")
		out("Exts: %v %v", l, err)
	})
}
