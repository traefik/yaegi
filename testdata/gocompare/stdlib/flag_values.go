// flag with program types: flag.Value and flag.Getter implementations,
// Func and BoolFunc, a FlagSet with
// ContinueOnError writing usage and errors to a buffer, Visit and VisitAll.
// (TextVar with a program type and a Value with IsBoolFlag are left out:
// they differ in yaegi v2 today.)
package main

import (
	"errors"
	"flag"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type List []string

func (l *List) String() string     { return strings.Join(*l, ",") }
func (l *List) Set(s string) error { *l = append(*l, strings.Split(s, ",")...); return nil }
func (l *List) Get() any           { return []string(*l) }

type Level int

func (l *Level) String() string {
	if l == nil {
		return "nil-level"
	}
	return []string{"debug", "info", "warn"}[*l]
}
func (l *Level) Set(s string) error {
	switch s {
	case "debug":
		*l = 0
	case "info":
		*l = 1
	case "warn":
		*l = 2
	default:
		return fmt.Errorf("bad level %q", s)
	}
	return nil
}

type Size int64

func (s *Size) UnmarshalText(b []byte) error {
	str := string(b)
	mult := int64(1)
	switch {
	case strings.HasSuffix(str, "K"):
		mult, str = 1024, strings.TrimSuffix(str, "K")
	case strings.HasSuffix(str, "M"):
		mult, str = 1024*1024, strings.TrimSuffix(str, "M")
	}
	n, err := strconv.ParseInt(str, 10, 64)
	if err != nil {
		return err
	}
	*s = Size(n * mult)
	return nil
}

func (s Size) MarshalText() ([]byte, error) {
	return []byte(strconv.FormatInt(int64(s), 10) + "B"), nil
}

type BoolCounter int

func (b *BoolCounter) String() string   { return strconv.Itoa(int(*b)) }
func (b *BoolCounter) Set(string) error { *b++; return nil }
func (b *BoolCounter) IsBoolFlag() bool { return true }

func parse(args []string) {
	var out strings.Builder
	fs := flag.NewFlagSet("prog", flag.ContinueOnError)
	fs.SetOutput(&out)
	var list List
	lvl := Level(1)
	var size Size
	var verbose BoolCounter
	var funcCalls []string
	fs.Var(&list, "item", "items to add")
	fs.Var(&lvl, "level", "log `level`")
	size = 1024
	name := fs.String("name", "anon", "the name")
	n := fs.Int("n", 3, "a number")
	d := fs.Duration("d", time.Second, "a duration")
	u := fs.Uint64("u", 0, "")
	f := fs.Float64("f", 0.5, "ratio")
	b := fs.Bool("b", false, "a bool")
	fs.Func("call", "a func flag", func(s string) error {
		if s == "fail" {
			return errors.New("func failed")
		}
		funcCalls = append(funcCalls, s)
		return nil
	})
	fs.BoolFunc("hook", "a bool func", func(s string) error { funcCalls = append(funcCalls, "hook="+s); return nil })
	err := fs.Parse(args)
	fmt.Printf("args=%q\n  err=%v help=%v\n", args, err, errors.Is(err, flag.ErrHelp))
	fmt.Printf("  list=%v level=%v size=%d verbose=%d name=%s n=%d d=%v u=%d f=%v b=%v calls=%v rest=%q nflag=%d nargs=%d\n",
		list, lvl.String(), size, verbose, *name, *n, *d, *u, *f, *b, funcCalls, fs.Args(), fs.NFlag(), fs.NArg())
	var set []string
	fs.Visit(func(fl *flag.Flag) { set = append(set, fl.Name+"="+fl.Value.String()) })
	fmt.Println("  set:", set)
	if g, ok := fs.Lookup("item").Value.(flag.Getter); ok {
		fmt.Printf("  getter: %#v\n", g.Get())
	}
	if out.Len() > 0 {
		fmt.Print("  output:\n", out.String())
	}
}

func main() {
	parse(nil)
	parse([]string{"-item", "a,b", "--item=c", "-level", "warn", "-name", "bob", "-n=-7", "-d", "1m30s", "-u", "18446744073709551615", "-f", "1e3", "-b", "-call", "x", "-call=y", "-hook", "rest", "-n", "9"})
	parse([]string{"-level", "loud"})
	parse([]string{"-size", "12Q"})
	parse([]string{"-n", "abc"})
	parse([]string{"-u", "-1"})
	parse([]string{"-nope"})
	parse([]string{"-call", "fail"})
	parse([]string{"-b=maybe"})
	parse([]string{"-name"})
	parse([]string{"--", "-n", "5"})
	parse([]string{"-h"})
	parse([]string{"-b=false", "-hook=1", "x", "y"})
	fs := flag.NewFlagSet("all", flag.ContinueOnError)
	fs.Var(new(Level), "lv", "")
	fs.Int("z", 0, "")
	fs.VisitAll(func(fl *flag.Flag) { fmt.Println(fl.Name, fl.DefValue, fl.Value) })
	fmt.Println(fs.Set("lv", "debug"), fs.Set("missing", "1"), fs.Lookup("lv").Value, fs.Name(), fs.Parsed())
	fmt.Println(flag.UnquoteUsage(fs.Lookup("z")))
}
