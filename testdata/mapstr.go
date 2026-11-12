package main

import "strconv"

type Item struct {
	Key   string
	Count int
}

func main() {
	m := map[string]int{}
	for i := 0; i < 1000000; i++ {
		k := "k" + strconv.Itoa(i%1000)
		m[k] += i
	}
	items := make([]Item, 0, len(m))
	for k, v := range m {
		items = append(items, Item{k, v})
	}
	total := 0
	for _, it := range items {
		total += it.Count % 7
	}
	println(len(items), total)
}
