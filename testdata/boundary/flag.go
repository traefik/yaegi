// Native boundary of flag: program types given to flag (docs/BOUNDARY.md).
package main

import (
	"flag"
	"fmt"
	"os"
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

// lines prints a text line by line.
func lines(title, text string) {
	for _, l := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		out("%s| %s", title, l)
	}
}

// intVal is a flag.Value with pointer receivers.
type intVal struct{ n int }

func (v *intVal) String() string { return fmt.Sprint(v.n) }
func (v *intVal) Set(s string) error {
	_, err := fmt.Sscan(s, &v.n)
	return err
}

// boolVal is a flag.Value with IsBoolFlag true.
type boolVal struct{ on bool }

func (v *boolVal) String() string { return fmt.Sprint(v.on) }
func (v *boolVal) Set(s string) error {
	v.on = s == "true"
	return nil
}
func (v *boolVal) IsBoolFlag() bool { return true }

// notBool has IsBoolFlag false: it needs a value.
type notBool struct{ s string }

func (v *notBool) String() string     { return v.s }
func (v *notBool) Set(s string) error { v.s = s; return nil }
func (v *notBool) IsBoolFlag() bool   { return false }

// listVal is a flag.Value with value receivers.
type listVal struct{ p *[]string }

func (v listVal) String() string {
	if v.p == nil {
		return ""
	}
	return strings.Join(*v.p, ",")
}
func (v listVal) Set(s string) error { *v.p = append(*v.p, s); return nil }

// gval is a generic flag.Value.
type gval[T any] struct{ v T }

func (g *gval[T]) String() string { return fmt.Sprint(g.v) }
func (g *gval[T]) Set(s string) error {
	_, err := fmt.Sscan(s, &g.v)
	return err
}

// panicVal panics in String on its zero value.
type panicVal struct{ p *string }

func (v panicVal) String() string     { return *v.p }
func (v panicVal) Set(s string) error { *v.p = s; return nil }

// ip4 is a text marshaler: value MarshalText, pointer UnmarshalText.
type ip4 [4]byte

func (ip ip4) MarshalText() ([]byte, error) {
	return []byte(fmt.Sprintf("%d.%d.%d.%d", ip[0], ip[1], ip[2], ip[3])), nil
}
func (ip *ip4) UnmarshalText(b []byte) error {
	_, err := fmt.Sscanf(string(b), "%d.%d.%d.%d", &ip[0], &ip[1], &ip[2], &ip[3])
	return err
}

// level is a text marshaler with pointer receivers only.
type level struct{ name string }

func (l *level) MarshalText() ([]byte, error) { return []byte("L:" + l.name), nil }
func (l *level) UnmarshalText(b []byte) error {
	if string(b) == "bad" {
		return fmt.Errorf("bad level")
	}
	l.name = string(b)
	return nil
}

// valText has value receivers only: it is not a pointer.
type valText struct{}

func (valText) MarshalText() ([]byte, error) { return []byte("v"), nil }
func (valText) UnmarshalText([]byte) error   { return nil }

// newSet returns a flag set with every kind of program Value.
func newSet(sb *strings.Builder) (*flag.FlagSet, *intVal, *boolVal, *notBool, *[]string, *gval[float64], *string) {
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	fs.SetOutput(sb)
	iv := &intVal{}
	bv := &boolVal{}
	nb := &notBool{}
	list := &[]string{}
	gv := &gval[float64]{}
	ps := new(string)
	fs.Var(iv, "i", "an `int` value")
	fs.Var(bv, "b", "a bool value")
	fs.Var(nb, "nb", "not a bool")
	fs.Var(listVal{list}, "l", "a list")
	fs.Var(gv, "g", "a generic value")
	fs.Var(panicVal{ps}, "p", "panics on zero")
	return fs, iv, bv, nb, list, gv, ps
}

func main() {
	// row: flag (*FlagSet).Parse assert *
	// row: flag (*FlagSet).Parse reflect *
	// row: flag (*FlagSet).Parse panic *
	probe("FlagSet.Parse", func() {
		var sb strings.Builder
		fs, iv, bv, nb, list, gv, ps := newSet(&sb)
		err := fs.Parse([]string{"-i", "7", "-b", "-nb", "x", "-l", "a", "-l=b", "-g", "2.5", "-p", "s", "rest"})
		out("err %v: i=%v b=%v nb=%v l=%v g=%v p=%v args=%v", err, iv.n, bv.on, nb.s, *list, gv.v, *ps, fs.Args())
		fs, _, bv, nb, _, _, _ = newSet(&sb)
		err = fs.Parse([]string{"-b=false", "-nb"})
		out("err %v: b=%v nb=%q", err, bv.on, nb.s)
		lines("usage", sb.String())
		sb.Reset()
		fs, _, _, _, _, _, _ = newSet(&sb)
		err = fs.Parse([]string{"-unknown"})
		out("unknown: %v", err)
		lines("unknown", sb.String())
	})
	// row: flag (*FlagSet).PrintDefaults assert *
	// row: flag (*FlagSet).PrintDefaults reflect *
	// row: flag (*FlagSet).PrintDefaults panic *
	// row: flag NewFlagSet assert *
	// row: flag NewFlagSet reflect *
	// row: flag NewFlagSet panic *
	probe("PrintDefaults", func() {
		var sb strings.Builder
		fs, iv, _, _, list, _, _ := newSet(&sb)
		iv.n = 3
		*list = []string{"x"}
		fs.PrintDefaults()
		lines("PrintDefaults", sb.String())
		sb.Reset()
		fs.Usage()
		lines("Usage", sb.String())
	})
	// row: flag UnquoteUsage assert x.(flag.boolFlag)
	probe("UnquoteUsage", func() {
		for _, f := range []*flag.Flag{
			{Name: "b", Usage: "a bool", Value: &boolVal{}},
			{Name: "nb", Usage: "not bool", Value: &notBool{}},
			{Name: "i", Usage: "an `int` here", Value: &intVal{}},
			{Name: "g", Usage: "gen", Value: &gval[int]{}},
			{Name: "l", Usage: "list", Value: listVal{}},
		} {
			name, usage := flag.UnquoteUsage(f)
			out("%s: %q %q", f.Name, name, usage)
		}
	})
	// row: flag (*FlagSet).TextVar reflect *
	// row: flag (*FlagSet).TextVar panic == of two interfaces: comparing uncomparable type
	probe("FlagSet.TextVar", func() {
		var sb strings.Builder
		fs := flag.NewFlagSet("t", flag.ContinueOnError)
		fs.SetOutput(&sb)
		var ip ip4
		var lv level
		fs.TextVar(&ip, "ip", ip4{1, 2, 3, 4}, "an ip")
		fs.TextVar(&lv, "lv", &level{"info"}, "a level")
		out("defaults: %v %v", ip, lv)
		err := fs.Parse([]string{"-ip", "10.0.0.1", "-lv", "debug"})
		out("parse: %v %v %v", err, ip, lv)
		err = fs.Parse([]string{"-lv", "bad"})
		out("bad: %v", err)
		fs.PrintDefaults()
		lines("defaults", sb.String())
		try := func(name string, f func()) {
			defer func() {
				if r := recover(); r != nil {
					out("%s: panic %T: %v", name, r, r)
				}
			}()
			f()
			out("%s: ok", name)
		}
		try("mismatch", func() { fs.TextVar(&ip, "ip2", &level{"x"}, "") })
		try("not pointer", func() { fs.TextVar(valText{}, "vt", valText{}, "") })
	})
	// row: flag TextVar reflect *
	// row: flag TextVar panic == of two interfaces: comparing uncomparable type
	probe("TextVar", func() {
		old := flag.CommandLine
		defer func() { flag.CommandLine = old }()
		var sb strings.Builder
		flag.CommandLine = flag.NewFlagSet("cmd", flag.ContinueOnError)
		flag.CommandLine.SetOutput(&sb)
		var ip ip4
		flag.TextVar(&ip, "ip", ip4{9, 8, 7, 6}, "an ip")
		out("default: %v", ip)
		err := flag.CommandLine.Parse([]string{"-ip=1.1.1.1"})
		out("parse: %v %v", err, ip)
		func() {
			defer func() {
				r := recover()
				out("mismatch: panic %T: %v", r, r)
			}()
			var lv level
			flag.TextVar(&lv, "lv", ip4{}, "")
		}()
	})
	// row: flag Parse assert *
	// row: flag Parse reflect *
	// row: flag Parse panic *
	// row: flag PrintDefaults assert *
	// row: flag PrintDefaults reflect *
	// row: flag PrintDefaults panic *
	probe("Parse", func() {
		old, oldArgs := flag.CommandLine, os.Args
		defer func() { flag.CommandLine, os.Args = old, oldArgs }()
		var sb strings.Builder
		fs, iv, bv, _, list, _, ps := newSet(&sb)
		flag.CommandLine = fs
		os.Args = []string{"prog", "-i", "5", "-b", "-l", "z", "-p", "q", "arg"}
		flag.Parse()
		out("i=%v b=%v l=%v p=%v args=%v parsed=%v", iv.n, bv.on, *list, *ps, flag.Args(), flag.Parsed())
		flag.PrintDefaults()
		lines("PrintDefaults", sb.String())
	})
}
