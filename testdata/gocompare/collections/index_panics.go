// Out-of-range index and slice expressions with variable indices: the
// recovered runtime errors print the same message as Go, for slices,
// arrays, pointers to arrays, strings, and 2- and 3-index slicing.
package main

import "fmt"

func try(name string, f func()) {
	defer func() {
		r := recover()
		if err, ok := r.(error); ok {
			fmt.Println(name, "error:", err.Error())
			return
		}
		fmt.Println(name, "value:", r)
	}()
	f()
}

func main() {
	s := []int{1, 2, 3}
	arr := [4]int{1, 2, 3, 4}
	pa := &arr
	str := "héllo"
	n, neg, big := 5, -1, 10
	i, j, k := 2, 1, 3

	try("s[n]", func() { _ = s[n] })
	try("s[neg]", func() { _ = s[neg] })
	try("s[len]", func() { _ = s[len(s)] })
	try("set s[n]", func() { s[n] = 1 })
	try("arr[n]", func() { _ = arr[n] })
	try("arr[neg]", func() { arr[neg] = 0 })
	try("pa[big]", func() { _ = pa[big] })
	try("str[n+1]", func() { _ = str[n+1] })
	try("str[neg]", func() { _ = str[neg] })
	var nilS []int
	try("nil[0]", func() { _ = nilS[j-1] })

	try("s[:n]", func() { _ = s[:n] })
	try("s[n:]", func() { _ = s[n:] })
	try("s[i:j]", func() { _ = s[i:j] })
	try("s[neg:]", func() { _ = s[neg:] })
	try("s[:neg]", func() { _ = s[:neg] })
	try("s[::]", func() { _ = s[0:1:n] })
	try("s[i:j:k]", func() { _ = s[i:j:k] })
	try("s[j:k:i]", func() { _ = s[j:k:i] })
	try("s[k:k:j]", func() { _ = s[k:k:j] })
	try("arr[:big]", func() { _ = arr[:big] })
	try("arr[i:j]", func() { _ = arr[i:j] })
	try("pa[:big]", func() { _ = pa[:big] })
	try("pa[1:2:big]", func() { _ = pa[1:2:big] })
	try("str[:big]", func() { _ = str[:big] })
	try("str[i:j]", func() { _ = str[i:j] })
	try("str[big:]", func() { _ = str[big:] })

	// Within cap but past len is fine for slicing.
	m := make([]int, 2, 5)
	fmt.Println(len(m[:5]), len(m[3:5]), cap(m[5:5]))
	try("m[6]", func() { _ = m[:n+1] })
	try("m[3]", func() { _ = m[k] })

	// Index of a nil pointer to array.
	var np *[3]int
	try("np[0]", func() { _ = np[j] })
	try("np len", func() { fmt.Println(len(np)) })
	try("np range", func() {
		for i := range np {
			fmt.Print(i)
		}
		fmt.Println()
	})
	try("np range v", func() {
		for _, v := range np {
			fmt.Print(v)
		}
	})

	// Large values with int64 and uint indices.
	var u uint = 7
	var i64 int64 = 1 << 40
	try("s[u]", func() { _ = s[u] })
	try("s[i64]", func() { _ = s[i64] })
	var u8 uint8 = 200
	try("s[u8]", func() { _ = s[u8] })
	try("s[u8:]", func() { _ = s[u8:] })

	// Index on a slice of slices.
	ss := [][]string{{"a"}, {}}
	try("ss[1][0]", func() { _ = ss[j][0] })
	try("ss[2]", func() { _ = ss[i][0] })
	fmt.Println("end")
}
