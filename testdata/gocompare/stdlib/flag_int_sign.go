// flag.IntVar and UintVar with variables of the program, on every
// platform: flag writes the variables itself. On a 32-bit platform (386,
// arm) an int is half of the VM word: a negative value written by flag must
// still compare, divide and convert as a negative int, also inside a
// callback that Parse runs, and after a positive value replaced it.
package main

import (
	"flag"
	"fmt"
	"math"
	"os"
)

func show(n int, u uint) {
	fmt.Println(n, n < 0, n/2, int64(n), n == -3, u, u > 4, uint64(u))
}

func main() {
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	var n int
	var u uint
	fs.IntVar(&n, "n", -1, "an int")
	fs.UintVar(&u, "u", 7, "a uint")
	show(n, u)

	fs.Func("cb", "prints n", func(string) error {
		fmt.Print("cb: ")
		show(n, u)
		return nil
	})
	fmt.Println(fs.Parse([]string{"-n=-3", "-u=5", "-cb=x"}))
	show(n, u)

	fmt.Println(fs.Set("n", "12"))
	show(n, u)
	fmt.Println(fs.Set("n", fmt.Sprint(math.MinInt32)))
	show(n, u)
	fs.Lookup("n").Value.Set("-7")
	show(n, u)
	fmt.Println(fs.Set("u", fmt.Sprint(uint32(math.MaxUint32))))
	show(n, u)

	// the program writes the variable, flag reads it
	n = -9
	fmt.Println(fs.Lookup("n").Value.String())
	n++
	fmt.Println(fs.Lookup("n").Value.(flag.Getter).Get(), n < -7)
}
