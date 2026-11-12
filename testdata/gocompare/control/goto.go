// goto: forward and backward jumps, loops built with goto, jumping out of
// nested blocks and loops, goto over a block to an outer label, goto inside
// switch and select cases, and state machines written with goto.
package main

import "fmt"

func lex(s string) []string {
	var toks []string
	i := 0
	start := 0
next:
	if i >= len(s) {
		goto end
	}
	if s[i] == ' ' {
		i++
		goto next
	}
	start = i
	if s[i] >= '0' && s[i] <= '9' {
		goto number
	}
	goto word
number:
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	toks = append(toks, "NUM("+s[start:i]+")")
	goto next
word:
	for i < len(s) && s[i] != ' ' {
		i++
	}
	toks = append(toks, "WORD("+s[start:i]+")")
	goto next
end:
	return toks
}

func search(m [][]int, t int) string {
	for i := range m {
		for j := range m[i] {
			if m[i][j] == t {
				goto found
			}
		}
	}
	return "not found"
found:
	return "found"
}

func retry(fail int) (attempts int, log []string) {
again:
	attempts++
	if attempts <= fail {
		log = append(log, fmt.Sprint("fail ", attempts))
		goto again
	}
	log = append(log, "ok")
	return
}

func inSwitch(n int) string {
	switch {
	case n < 0:
		goto neg
	case n == 0:
		return "zero"
	}
	return "pos"
neg:
	return "neg"
}

func gotoDefer() (out []int) {
	i := 0
loop:
	if i < 3 {
		defer func(v int) { out = append(out, v) }(i)
		i++
		goto loop
	}
	return []int{-1}
}

func main() {
	fmt.Println(lex("abc 123 x9  45 end"))
	fmt.Println(lex(""))
	fmt.Println(lex("   "))
	m := [][]int{{1, 2}, {3, 4}}
	fmt.Println(search(m, 4), search(m, 5))
	fmt.Println(retry(0))
	fmt.Println(retry(3))
	fmt.Println(inSwitch(-1), inSwitch(0), inSwitch(5))
	fmt.Println(gotoDefer())
	ch := make(chan int, 1)
	ch <- 1
	select {
	case v := <-ch:
		if v == 1 {
			goto selected
		}
	default:
	}
	fmt.Println("never")
selected:
	fmt.Println("selected")
	n := 0
	sum := 0
top:
	{
		n++
		sum += n
		if n < 100 {
			goto top
		}
	}
	fmt.Println(n, sum)
	var states []string
	state := 0
machine:
	switch state {
	case 0:
		states = append(states, "start")
		state = 2
		goto machine
	case 1:
		states = append(states, "one")
		state = 3
		goto machine
	case 2:
		states = append(states, "two")
		state = 1
		goto machine
	}
	states = append(states, "stop")
	fmt.Println(states)
}
