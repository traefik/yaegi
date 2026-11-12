// The product of two complex numbers rounds each partial product: the VM
// never uses a fused multiply-add (arm64), so a*a gives the digits of gc's
// constant folding, and b*b gives NaN, not +Inf, for the real part.
// The operands stay local constants that gc folds: with operands known
// only at run time, gc on arm64 fuses too.
package main

import "fmt"

func main() {
	a := complex(1e150, 2e150)
	b := complex(1e300, 1e300)
	fmt.Println(a*a, b*b)
	c := complex64(complex(1e20, 3e19))
	fmt.Println(c * c)
	x := complex(1.1, 2.2)
	x *= complex(3.3, -4.4)
	fmt.Println(x)
}
