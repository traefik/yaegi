// Switches on integers: the VM turns the chain of tests that ssa makes
// of them into one jump (vm/switches.go). Dense and sparse keys, negative
// and large keys, small and unsigned types, values written in the cases
// and read after the switch (phis), fallthrough, no default, and a
// switch in a loop that continues.
package main

import "fmt"

func dense(x int) string {
	switch x {
	case 0:
		return "zero"
	case 1:
		return "one"
	case 2:
		return "two"
	case 3:
		return "three"
	case 5:
		return "five"
	case 6, 7:
		return "six-seven"
	default:
		return "other"
	}
}

func sparse(x int64) int {
	n := 0
	switch x {
	case -1 << 40:
		n = 1
	case -7:
		n = 2
	case 0:
		n = 3
	case 1000:
		n = 4
	case 1 << 50:
		n = 5
	case 1<<63 - 1:
		n = 6
	}
	return n * 10
}

func unsigned(x uint64) string {
	switch x {
	case 1<<64 - 1:
		return "max"
	case 1 << 63:
		return "half"
	case 0:
		return "zero"
	case 42:
		return "42"
	}
	return "none"
}

func small(x int8) (s string) {
	switch x {
	case -128:
		s = "min"
	case -1:
		s = "minus one"
	case 0:
		s = "zero"
	case 127:
		s = "max"
	default:
		s = fmt.Sprint("int8 ", x)
	}
	return s + "!"
}

func through(x uint8) []string {
	var out []string
	switch x {
	case 1:
		out = append(out, "1")
		fallthrough
	case 2:
		out = append(out, "2")
	case 3:
		out = append(out, "3")
		fallthrough
	case 4:
		out = append(out, "4")
		fallthrough
	default:
		out = append(out, "d")
	}
	return out
}

type Kind int

const (
	KA Kind = iota
	KB
	KC
	KD
	KE
)

func (k Kind) String() string {
	switch k {
	case KA:
		return "A"
	case KB:
		return "B"
	case KC:
		return "C"
	case KD:
		return "D"
	}
	return fmt.Sprintf("Kind(%d)", int(k))
}

func loop(xs []int) (sum int) {
	for _, x := range xs {
		switch x {
		case 1:
			continue
		case 2:
			sum += 2
		case 3:
			sum += 30
		case 4:
			sum += 400
		case 9:
			break
		}
		sum++
	}
	return sum
}

func main() {
	for x := -2; x < 10; x++ {
		fmt.Print(dense(x), " ")
	}
	fmt.Println()
	for _, x := range []int64{-1 << 40, -7, 0, 1000, 1 << 50, 1<<63 - 1, 3, -1 << 63} {
		fmt.Print(sparse(x), " ")
	}
	fmt.Println()
	for _, x := range []uint64{1<<64 - 1, 1 << 63, 0, 42, 7} {
		fmt.Print(unsigned(x), " ")
	}
	fmt.Println()
	for _, x := range []int8{-128, -1, 0, 127, 5} {
		fmt.Print(small(x), " ")
	}
	fmt.Println()
	for x := uint8(0); x < 6; x++ {
		fmt.Print(through(x), " ")
	}
	fmt.Println()
	for k := Kind(-1); k < 7; k++ {
		fmt.Print(k, " ")
	}
	fmt.Println()
	fmt.Println(loop([]int{1, 2, 3, 4, 5, 9, 1, 2}))
}
