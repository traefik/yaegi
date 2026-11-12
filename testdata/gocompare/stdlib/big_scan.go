// fmt scanning calls the Scan method of *big.Int, *big.Float and *big.Rat.
package main

import (
	"fmt"
	"math/big"
)

func main() {
	a := new(big.Int)
	n, err := fmt.Sscan("18446744073709551617", a)
	fmt.Println(n, a, err)
	f := new(big.Float)
	n, err = fmt.Sscan("1.5e3", f)
	fmt.Println(n, f, err)
	r := new(big.Rat)
	n, err = fmt.Sscan("3/2", r)
	fmt.Println(n, r, err)
	var x, y big.Int
	n, err = fmt.Sscanf("0x1f -12", "%v %d", &x, &y)
	fmt.Println(n, &x, &y, err)
	n, err = fmt.Sscan("abc", a)
	fmt.Println(n, a, err)
}
