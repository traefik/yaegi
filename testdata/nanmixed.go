package main

import (
	"fmt"
	"math"
	"reflect"
	"sort"
)

// A map[any]V with float32 and float64 NaN keys: each NaN entry keeps its
// own key and value in range, reflect and native conversions.

func kind(k any) string {
	switch k.(type) {
	case float32:
		return "f32"
	case float64:
		return "f64"
	}
	return "other"
}

func check(name string, pairs [][2]string) {
	bad := 0
	for _, p := range pairs {
		if p[0] != p[1] && p[1] != "x" {
			bad++
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i][1] != pairs[j][1] {
			return pairs[i][1] < pairs[j][1]
		}
		return pairs[i][0] < pairs[j][0]
	})
	fmt.Println(name, len(pairs), "mismatches:", bad, pairs)
}

func main() {
	m := map[any]string{}
	for i := 0; i < 4; i++ {
		m[float32(math.NaN())] = "f32"
		m[math.NaN()] = "f64"
	}
	m["s"] = "x"
	fmt.Println(len(m), m[math.NaN()] == "")

	var pairs [][2]string
	for k, v := range m {
		pairs = append(pairs, [2]string{kind(k), v})
	}
	check("range", pairs)

	pairs = nil
	it := reflect.ValueOf(m).MapRange()
	for it.Next() {
		pairs = append(pairs, [2]string{kind(it.Key().Interface()), it.Value().String()})
	}
	check("reflect", pairs)

	pairs = nil
	for _, k := range reflect.ValueOf(m).MapKeys() {
		pairs = append(pairs, [2]string{kind(k.Interface()), "x"})
	}
	check("keys", pairs)

	delete(m, math.NaN())
	fmt.Println(len(m))
	clear(m)
	fmt.Println(len(m))
}
