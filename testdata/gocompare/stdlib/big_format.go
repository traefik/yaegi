// fmt calls the Format method of *big.Int, *big.Float and *big.Rat.
package main

import (
	"errors"
	"fmt"
	"math/big"
)

func main() {
	a := big.NewInt(255)
	fmt.Printf("%x %d %v %s\n", a, a, a, a)
	fmt.Println(a, fmt.Sprint(a))
	fmt.Printf("%08d|%+d|%X|%o|%b|%#x|%-6d|\n", a, a, a, a, a, a, a)
	f := big.NewFloat(1.5)
	fmt.Printf("%.3f %v %g\n", f, f, f)
	fmt.Println(f)
	fmt.Printf("%10.2f|%-10.1e|%G\n", f, f, f)
	r := big.NewRat(3, 4)
	fmt.Printf("%v %s\n", r, r)
	var n *big.Int
	fmt.Printf("%d %v\n", n, n)
	xs := []any{a, f, r}
	fmt.Printf("%v %d\n", xs, []*big.Int{a, big.NewInt(-7)})
	err := fmt.Errorf("value %d: %w", a, errors.New("bad"))
	fmt.Println(err, errors.Unwrap(err))
	fmt.Println(fmt.Sprintf("%T %T", a, f))
}
