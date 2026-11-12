// flag.BoolVar, IntVar, DurationVar... and the methods of *flag.FlagSet
// with variables of the program: flag writes the variables themselves (it
// wrote a native copy of them), so Parse, Set, a Value.Set and the
// callbacks that Parse runs all see the same variable.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"
)

var gb bool

type opts struct {
	n int
	d time.Duration
}

func main() {
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	var (
		b   bool
		n   int
		i64 int64
		u   uint
		u64 uint64
		f   float64
		o   opts
	)
	fs.BoolVar(&b, "b", false, "a bool")
	fs.IntVar(&n, "n", 7, "an int")
	fs.Int64Var(&i64, "i64", 0, "an int64")
	fs.UintVar(&u, "u", 0, "a uint")
	fs.Uint64Var(&u64, "u64", 3, "a uint64")
	fs.Float64Var(&f, "f", 1.5, "a float")
	fs.DurationVar(&o.d, "d", time.Second, "a duration")
	fs.IntVar(&o.n, "on", 0, "a field")
	fs.BoolVar(&gb, "g", false, "a global")
	fmt.Println("defaults:", b, n, i64, u, u64, f, o, gb)

	// a callback of the program run by Parse sees the flags parsed before
	fs.Func("show", "prints n", func(s string) error {
		fmt.Println("show:", s, n, b)
		return nil
	})
	err := fs.Parse([]string{"-b", "-n=3", "-show=x", "-i64=-4", "-u=5", "-u64=6", "-f=2.25", "-d=1m", "-on=9", "-g", "rest"})
	fmt.Println("parse:", err, b, n, i64, u, u64, f, o, gb, fs.Args())

	fmt.Println(fs.Set("n", "11"), n, fs.Lookup("n").Value.String(), fs.Lookup("n").DefValue)
	fmt.Println(fs.Set("n", "x"))
	fmt.Println(fs.Set("b", "maybe"), b)
	n = 42
	fmt.Println(fs.Lookup("n").Value.String(), fs.Lookup("n").Value.(flag.Getter).Get())
	v := fs.Lookup("n").Value
	n = 20
	v.Set("30")
	fmt.Println("Value.Set:", n)
	fmt.Printf("%T %T\n", fs.Lookup("d").Value.(flag.Getter).Get(), fs.Lookup("f").Value.(flag.Getter).Get())
	fs.PrintDefaults()

	// one variable bound to two flags
	var x int
	fs2 := flag.NewFlagSet("two", flag.ContinueOnError)
	fs2.IntVar(&x, "a", 1, "")
	fs2.IntVar(&x, "b", 2, "")
	fmt.Println(fs2.Parse([]string{"-a", "5", "-b", "7"}), x)

	var top int
	flag.IntVar(&top, "top", 8, "a flag of CommandLine")
	fmt.Println(top, flag.Lookup("top").Value)
}
