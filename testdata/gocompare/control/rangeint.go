// range over integers: typed and untyped bounds, zero and negative counts,
// the type of the variable, changes of the bound or of the variable in the
// body, and break/continue.
package main

import "fmt"

type myInt int16

func main() {
	for i := range 5 {
		fmt.Print(i, " ")
	}
	fmt.Println()
	n := 0
	for range 7 {
		n++
	}
	fmt.Println("n", n)
	for i := range 0 {
		fmt.Println("never", i)
	}
	neg := -3
	for i := range neg {
		fmt.Println("never", i)
	}
	var u8 uint8 = 4
	for i := range u8 {
		fmt.Printf("%T:%v ", i, i)
	}
	fmt.Println()
	var m myInt = 3
	for i := range m {
		fmt.Printf("%T:%v ", i, i)
	}
	fmt.Println()
	for i := range int64(3) {
		fmt.Printf("%T ", i)
	}
	fmt.Println()
	bound := 4
	for i := range bound {
		bound = 100 // evaluated once
		fmt.Print(i, " ")
	}
	fmt.Println("bound", bound)
	for i := range 6 {
		i *= 10 // does not change the iteration count
		fmt.Print(i, " ")
	}
	fmt.Println()
	for i := range 10 {
		if i%3 == 0 {
			continue
		}
		if i > 7 {
			break
		}
		fmt.Print(i, " ")
	}
	fmt.Println()
	var j int
	for j = range 4 {
	}
	fmt.Println("j", j)
	total := int64(0)
	for i := range int64(1000) {
		total += i
	}
	fmt.Println("total", total)
	const c = 3
	for i := range c {
		fmt.Printf("%T%v ", i, i)
	}
	fmt.Println()
	var f float64
	for i := range 3 {
		f += float64(i) / 2
	}
	fmt.Println(f)
	s := []string{"a", "b", "c"}
	for i := range len(s) {
		fmt.Print(s[len(s)-1-i])
	}
	fmt.Println()
	for i := range 3 {
		for j := range i {
			fmt.Print(i, j, " ")
		}
	}
	fmt.Println()
}
