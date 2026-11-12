// flag.StringVar and flag.TextVar with variables of the program, and the
// methods of *flag.FlagSet called with defer and go: flag writes the
// variables of the program. PrintDefaults is not printed: with StringVar
// it shows "-s value" where Go shows "-s string" (docs of flagvars.go).
package main

import (
	"flag"
	"fmt"
	"io"
	"net"
	"strings"
)

type level struct{ name string }

func (l *level) MarshalText() ([]byte, error) { return []byte("L:" + l.name), nil }
func (l *level) UnmarshalText(b []byte) error {
	if string(b) == "bad" {
		return fmt.Errorf("bad level")
	}
	l.name = string(b)
	return nil
}

func main() {
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var s, s2 string
	var ip net.IP
	var lv level
	fs.StringVar(&s, "s", "def", "a string")
	fs.StringVar(&s2, "a", "x", "")
	fs.StringVar(&s2, "b", "y", "")
	fs.TextVar(&ip, "ip", net.IPv4(1, 2, 3, 4), "an ip")
	fs.TextVar(&lv, "lv", &level{"info"}, "a level")
	fmt.Println("defaults:", s, s2, ip, lv, fs.Lookup("s").DefValue, fs.Lookup("ip").DefValue, fs.Lookup("lv").DefValue)
	err := fs.Parse([]string{"-s=x", "-a", "first", "-b", "second", "-ip", "10.0.0.1", "-lv", "debug"})
	fmt.Println("parse:", err, s, s2, ip, lv)
	fmt.Println(fs.Parse([]string{"-lv", "bad"}), lv)
	s = "program"
	fmt.Println(fs.Lookup("s").Value, fs.Set("s", "set"), s)
	fmt.Println(fs.Lookup("ip").Value, fs.Lookup("lv").Value)
	if g, ok := fs.Lookup("s").Value.(flag.Getter); ok {
		fmt.Println("getter:", g.Get())
	}
	// PrintDefaults calls String on a zero Value of the flag (isZeroValue)
	var out strings.Builder
	fs.SetOutput(&out)
	fs.PrintDefaults()
	fs.SetOutput(io.Discard)
	fmt.Println("defaults text:", strings.Contains(out.String(), "panic"), strings.Contains(out.String(), "def"))

	try := func(name string, f func()) {
		defer func() { fmt.Println(name+":", recover()) }()
		f()
	}
	try("mismatch", func() { fs.TextVar(&lv, "lv2", net.IP{}, "") })
	try("mismatch2", func() { fs.TextVar(&ip, "ip2", &level{}, "") })

	var n int
	var name string
	func() {
		defer fs.IntVar(&n, "n", 5, "")
		defer fs.StringVar(&name, "name", "anon", "")
	}()
	done := make(chan bool)
	var g string
	go func() {
		defer close(done)
		fs.StringVar(&g, "g", "go", "")
	}()
	<-done
	fmt.Println("defer:", n, name, g)
	fmt.Println(fs.Parse(strings.Fields("-n 7 -name bob -g x")), n, name, g)

	var top string
	flag.StringVar(&top, "top", "cmdline", "a flag of CommandLine")
	flag.Set("top", "changed")
	fmt.Println(top, flag.Lookup("top").Value)
}
