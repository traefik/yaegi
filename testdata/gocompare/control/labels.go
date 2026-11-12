// Labeled break and continue across nested for, switch, select and range
// loops, and goto forward and backward, including goto out of blocks.
package main

import "fmt"

func find(grid [][]int, target int) (int, int) {
	r, c := -1, -1
outer:
	for i, row := range grid {
		for j, v := range row {
			if v == target {
				r, c = i, j
				break outer
			}
		}
	}
	return r, c
}

func gotoLoop(n int) []int {
	var out []int
	i := 0
loop:
	if i < n {
		out = append(out, i*i)
		i++
		goto loop
	}
	return out
}

func gotoForward(x int) string {
	if x < 0 {
		goto negative
	}
	if x == 0 {
		goto zero
	}
	return "positive"
negative:
	return "negative"
zero:
	return "zero"
}

func main() {
	grid := [][]int{{1, 2, 3}, {4, 5, 6}, {7, 8, 9}}
	for _, t := range []int{1, 5, 9, 10} {
		r, c := find(grid, t)
		fmt.Println(t, r, c)
	}
outer:
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			if j == 2 {
				continue outer
			}
			if i == 3 {
				break outer
			}
			fmt.Print(i, j, " ")
		}
	}
	fmt.Println()
sw:
	for i := 0; i < 5; i++ {
		switch i {
		case 1:
			continue sw
		case 3:
			break sw
		}
		fmt.Print("i=", i, " ")
	}
	fmt.Println()
	ch := make(chan int, 10)
	for i := 0; i < 10; i++ {
		ch <- i
	}
	done := make(chan bool)
	got := 0
sel:
	for {
		select {
		case v := <-ch:
			if v%2 == 0 {
				continue sel
			}
			if v == 7 {
				break sel
			}
			got += v
		case <-done:
			break sel
		}
	}
	fmt.Println("got", got, len(ch))
	fmt.Println(gotoLoop(6))
	for _, x := range []int{-3, 0, 4} {
		fmt.Print(gotoForward(x), " ")
	}
	fmt.Println()
	count := 0
L1:
	for i := 0; i < 3; i++ {
	L2:
		for j := 0; j < 3; j++ {
			for k := 0; k < 3; k++ {
				count++
				if k == 1 {
					continue L2
				}
				if j == 2 {
					continue L1
				}
			}
		}
	}
	fmt.Println("count", count)
	m := map[int]string{1: "a", 2: "b", 3: "c"}
	keysSeen := 0
R:
	for range m {
		for range 5 {
			keysSeen++
			if keysSeen%2 == 0 {
				continue R
			}
		}
	}
	fmt.Println("keysSeen", keysSeen)
	i := 0
back:
	i++
	if i < 5 {
		goto back
	}
	fmt.Println("i", i)
	{
		j := 0
	inner:
		if j < 3 {
			j++
			goto inner
		}
		fmt.Println("j", j)
	}
	x := 0
	for {
		x++
		if x > 3 {
			goto out
		}
	}
out:
	fmt.Println("x out", x)
	var labelsLog []string
L:
	for i := 0; ; i++ {
		switch {
		case i < 2:
			labelsLog = append(labelsLog, "small")
		case i < 4:
			labelsLog = append(labelsLog, "mid")
			continue L
		default:
			break L
		}
		labelsLog = append(labelsLog, "end-of-body")
	}
	fmt.Println(labelsLog)
}
