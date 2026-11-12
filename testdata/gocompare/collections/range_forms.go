// Range forms: over arrays, pointers to arrays, slices, strings, integers
// of several types, with index only, with blank, with no variables, and
// the per-iteration loop variables captured by closures.
package main

import "fmt"

type Count uint8

func main() {
	arr := [4]string{"a", "b", "c", "d"}
	for i := range arr {
		fmt.Print(i)
	}
	fmt.Println()
	for i, v := range &arr {
		fmt.Print(i, v)
	}
	fmt.Println()
	for _, v := range arr[1:3] {
		fmt.Print(v)
	}
	fmt.Println()
	n := 0
	for range arr {
		n++
	}
	fmt.Println(n)

	// The range expression of an array pointer is not copied.
	parr := &[3]int{1, 2, 3}
	for i, v := range parr {
		if i == 0 {
			parr[2] = 30
		}
		fmt.Print(v, " ")
	}
	fmt.Println()

	// Range over integers of several types.
	for i := range 3 {
		fmt.Print(i, " ")
	}
	var c Count = 4
	for i := range c {
		fmt.Printf("%T:%d ", i, i)
	}
	var i64 int64 = 3
	for i := range i64 {
		fmt.Printf("%T:%d ", i, i)
	}
	for i := range 0 {
		fmt.Print("never", i)
	}
	neg := -5
	for i := range neg {
		fmt.Print("never", i)
	}
	for range 2 {
		fmt.Print("x")
	}
	fmt.Println()
	// The bound is evaluated once.
	lim := 3
	for i := range lim {
		lim = 10
		fmt.Print(i)
	}
	fmt.Println(lim)

	// Strings: byte offsets and runes, invalid UTF-8.
	for i, r := range "aé日\xffz" {
		fmt.Print(i, ":", string(r), "/", r, " ")
	}
	fmt.Println()
	for i := range "héllo" {
		fmt.Print(i)
	}
	fmt.Println()

	// Per-iteration variables (Go 1.22).
	var fs []func() int
	for i := range 3 {
		fs = append(fs, func() int { return i })
	}
	for _, v := range []int{10, 20} {
		fs = append(fs, func() int { return v })
	}
	for i := 0; i < 2; i++ {
		fs = append(fs, func() int { return i * 100 })
	}
	for _, f := range fs {
		fmt.Print(f(), " ")
	}
	fmt.Println()
	var ptrs []*int
	for i := range 3 {
		ptrs = append(ptrs, &i)
	}
	fmt.Println(*ptrs[0], *ptrs[1], *ptrs[2])

	// Range over a map with index only, values summed.
	m := map[string]int{"a": 1, "b": 2, "c": 3}
	sum := 0
	for k := range m {
		sum += m[k]
	}
	fmt.Println(sum)

	// Range over a channel.
	ch := make(chan int, 5)
	for i := range 5 {
		ch <- i * i
	}
	close(ch)
	for v := range ch {
		fmt.Print(v, " ")
	}
	fmt.Println()

	// Assigning to existing variables in range.
	var idx int
	var val string
	for idx, val = range []string{"p", "q"} {
	}
	fmt.Println(idx, val)
	var arr2 [3]int
	for idx, arr2[idx] = range []int{7, 8, 9} {
	}
	fmt.Println(arr2)
	// Range over an array value returned by a function.
	get := func() [3]int { return [3]int{4, 5, 6} }
	for i, v := range get() {
		fmt.Print(i*v, " ")
	}
	fmt.Println()
	// Range over a nil slice, nil map and nil array pointer with index only.
	var ns []int
	var nm map[int]int
	var np *[2]int
	for i := range ns {
		fmt.Print(i)
	}
	for k := range nm {
		fmt.Print(k)
	}
	for i := range np {
		fmt.Print(i)
	}
	fmt.Println("done")
}
