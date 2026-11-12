// AOT test program: flag with interpreted flag.Value implementations.
//
//aot:lib flag
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// List is a flag.Value collecting repeated values.
type List []string

func (l *List) String() string     { return strings.Join(*l, ",") }
func (l *List) Set(v string) error { *l = append(*l, v); return nil }

// Level is a flag.Value with validation and a getter.
type Level int

func (l *Level) String() string { return strconv.Itoa(int(*l)) }
func (l *Level) Set(v string) error {
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 || n > 3 {
		return errors.New("level must be 0..3")
	}
	*l = Level(n)
	return nil
}
func (l *Level) Get() any { return int(*l) }

// Switch is a boolean flag.
type Switch bool

func (s *Switch) String() string     { return strconv.FormatBool(bool(*s)) }
func (s *Switch) Set(v string) error { b, err := strconv.ParseBool(v); *s = Switch(b); return err }
func (s *Switch) IsBoolFlag() bool   { return true }

func main() {
	if benchMain(benchOp) {
		return
	}
	fs := flag.NewFlagSet("prog", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	var tags List
	lvl := Level(1)
	var sw Switch
	name := fs.String("name", "anon", "user `name`")
	n := fs.Int("n", 3, "count")
	d := fs.Duration("d", time.Second, "delay")
	fs.Var(&tags, "tag", "a tag (repeatable)")
	fs.Var(&lvl, "level", "verbosity")
	fs.Var(&sw, "v", "verbose switch")
	var seen []string
	fs.Func("hook", "callback flag", func(s string) error { seen = append(seen, strings.ToUpper(s)); return nil })
	fs.TextVar(new(net), "ip", net{}, "an address")
	err := fs.Parse([]string{"-name=bob", "-tag", "a", "-tag=b", "-level", "2", "-v", "-hook", "x", "-n", "7", "-d", "2m", "rest", "-x"})
	fmt.Println(err, *name, *n, *d, tags, lvl, sw, seen, fs.Args(), fs.NArg())
	fs.Visit(func(f *flag.Flag) { fmt.Printf("set %s=%s;", f.Name, f.Value) })
	fmt.Println()
	if g, ok := fs.Lookup("level").Value.(flag.Getter); ok {
		fmt.Printf("getter %v %T\n", g.Get(), g.Get())
	}
	fs.PrintDefaults()
	err = fs.Parse([]string{"-level", "9"})
	fmt.Println("err:", err)
	err = fs.Parse([]string{"-unknown"})
	fmt.Println("err:", err)
	fmt.Println(fs.Set("tag", "c"), tags.String())
}

type net struct{ a, b int }

func (n net) MarshalText() ([]byte, error) { return []byte(fmt.Sprintf("%d.%d", n.a, n.b)), nil }
func (n *net) UnmarshalText(b []byte) error {
	_, err := fmt.Sscanf(string(b), "%d.%d", &n.a, &n.b)
	return err
}

// benchOp: parse a small command line with custom values.
func benchOp() {
	fs := flag.NewFlagSet("b", flag.ContinueOnError)
	var tags List
	lvl := Level(0)
	fs.Var(&tags, "tag", "")
	fs.Var(&lvl, "level", "")
	n := fs.Int("n", 0, "")
	if err := fs.Parse([]string{"-tag", "a", "-tag", "b", "-level", "2", "-n", "3"}); err != nil || *n != 3 {
		panic(err)
	}
}

func benchMain(op func()) bool {
	n, _ := strconv.Atoi(os.Getenv("AOT_BENCH"))
	if n <= 0 {
		return false
	}
	op()
	t0 := time.Now()
	for i := 0; i < n; i++ {
		op()
	}
	fmt.Printf("BENCH %d\n", time.Since(t0).Nanoseconds()/int64(n))
	return true
}
