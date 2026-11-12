package main

import (
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
)

// Range over native maps, VM maps and strings. The VM reuses one result
// tuple per loop, reads the scalar keys and values of a native map through
// one slot, and skips the conversion of a component the loop does not read:
// the values must still be the ones of Go, also when they are kept after
// the turn of the loop.

func main() {
	h := http.Header{}
	for _, k := range []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J"} {
		h.Set("X-"+k, "v-"+strings.ToLower(k))
	}
	h.Add("X-A", "second")

	// Keys only.
	var keys []string
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Println(keys)

	// Values only, kept after the loop.
	var vals [][]string
	for _, v := range h {
		vals = append(vals, v)
	}
	var flat []string
	for _, v := range vals {
		flat = append(flat, strings.Join(v, "+"))
	}
	sort.Strings(flat)
	fmt.Println(flat)

	// Keys and values; a value shares its array with the map, as in Go.
	for k, v := range h {
		if k == "X-B" {
			v[0] = "changed"
		}
	}
	fmt.Println(h.Get("X-B"))

	// Nested ranges over the same native map.
	pairs := 0
	for k1 := range h {
		for k2, v2 := range h {
			if k1 == k2 && len(v2) > 0 {
				pairs++
			}
		}
	}
	fmt.Println(pairs)

	// Closures and goroutines that keep the key and the value.
	var mu sync.Mutex
	var wg sync.WaitGroup
	var got []string
	for k, v := range h {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mu.Lock()
			got = append(got, k+"="+v[0])
			mu.Unlock()
		}()
	}
	wg.Wait()
	sort.Strings(got)
	fmt.Println(got)

	// A native map[string]string and url.Values.
	_, params, err := mime.ParseMediaType("text/html; charset=utf-8; q=0.5; level=1")
	if err != nil {
		panic(err)
	}
	var ps []string
	for k, v := range params {
		ps = append(ps, k+":"+v)
	}
	sort.Strings(ps)
	fmt.Println(ps)
	q, _ := url.ParseQuery("a=1&b=2&a=3&c=")
	var qs []string
	for k, v := range q {
		qs = append(qs, fmt.Sprint(k, v))
	}
	sort.Strings(qs)
	fmt.Println(qs)
	n := 0
	for range q {
		n++
	}
	fmt.Println(n)

	// Strings: positions and runes, kept after the loop, nested, with break.
	type pr struct {
		i int
		r rune
	}
	var rs []pr
	for i, r := range "héllo, 世界" {
		rs = append(rs, pr{i, r})
	}
	fmt.Println(rs)
	cnt := 0
	for i := range "abc" {
		for j, r := range "xyz" {
			if j == 2 {
				break
			}
			cnt += i + int(r)
		}
	}
	fmt.Println(cnt)

	// VM maps: pairs kept after the loop, nested ranges.
	vm := map[string]int{"a": 1, "b": 2, "c": 3, "d": 4}
	type kv struct {
		k string
		v int
	}
	var kvs []kv
	for k, v := range vm {
		kvs = append(kvs, kv{k, v})
	}
	sort.Slice(kvs, func(i, j int) bool { return kvs[i].k < kvs[j].k })
	fmt.Println(kvs)
	total := 0
	for _, v1 := range vm {
		for k2, v2 := range vm {
			if k2 != "" {
				total += v1 * v2
			}
		}
	}
	fmt.Println(total)
}
