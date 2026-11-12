// switch statements: no condition, init statement, case lists, fallthrough,
// break inside a case, default in the middle, duplicate-free evaluation of
// case expressions in order, switch on strings and runes.
package main

import "fmt"

var trace []string

func val(name string, v int) int {
	trace = append(trace, name)
	return v
}

func grade(n int) string {
	switch {
	case n >= 90:
		return "A"
	case n >= 80:
		return "B"
	case n >= 70:
		return "C"
	default:
		return "F"
	}
}

func kind(r rune) string {
	switch r {
	case 'a', 'e', 'i', 'o', 'u':
		return "vowel"
	case ' ', '\t', '\n':
		return "space"
	default:
		if r >= '0' && r <= '9' {
			return "digit"
		}
		return "other"
	}
}

func fall(n int) []string {
	var out []string
	switch n {
	case 0:
		out = append(out, "zero")
		fallthrough
	case 1:
		out = append(out, "one")
		fallthrough
	case 2:
		out = append(out, "two")
	case 3:
		out = append(out, "three")
		fallthrough
	default:
		out = append(out, "default")
	case 4:
		out = append(out, "four")
		fallthrough
	case 5:
		out = append(out, "five")
	}
	return out
}

func main() {
	for _, n := range []int{95, 85, 75, 10, 90, 80} {
		fmt.Print(grade(n), " ")
	}
	fmt.Println()
	for _, r := range "ai 7z\n" {
		fmt.Print(kind(r), " ")
	}
	fmt.Println()
	for n := -1; n <= 6; n++ {
		fmt.Println(n, fall(n))
	}
	switch x := val("init", 5); val("tag", x+1) {
	case val("c1", 1), val("c2", 6), val("c3", 6):
		fmt.Println("matched", x)
	case val("c4", 6):
		fmt.Println("never")
	}
	fmt.Println(trace)
	trace = nil
	switch val("tag", 9) {
	case val("a", 1):
	case val("b", 2), val("c", 3):
	default:
		trace = append(trace, "default")
	}
	fmt.Println(trace)
	for i := 0; i < 5; i++ {
		switch {
		case i == 1:
			fmt.Println("i==1, break switch")
			break
		case i == 3:
			if i > 2 {
				fmt.Println("i==3, break early")
				break
			}
			fmt.Println("never")
		default:
			fmt.Println("default", i)
		}
	}
	switch s := "go-vm"; s {
	case "go":
		fmt.Println("go")
	case "go-vm":
		fmt.Println("go-vm", len(s))
	}
	switch {
	}
	switch x := 3; {
	case x > 2:
		fmt.Println("x>2")
	}
	type color int
	const (
		red color = iota
		green
		blue
	)
	for _, c := range []color{blue, red, green, 7} {
		switch c {
		case red:
			fmt.Print("red ")
		case green, blue:
			fmt.Print("green-or-blue ")
		default:
			fmt.Print("unknown ", int(c), " ")
		}
	}
	fmt.Println()
	var iface interface{} = 3
	switch iface {
	case "3":
		fmt.Println("string 3")
	case 3.0:
		fmt.Println("float 3")
	case 3:
		fmt.Println("int 3")
	}
	switch b := true; b {
	case false:
		fmt.Println("false")
	case true:
		fmt.Println("true")
	}
}
