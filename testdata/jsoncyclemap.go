package main

// json.Marshal of maps and slices that contain themselves through an
// interface: encoding/json returns an UnsupportedValueError ("encountered
// a cycle via map[string]interface {}"). In default mode, the conversion of
// the value to native data for the native json never ended and the host
// died with a fatal Go stack overflow (convGraph.enterComp in vm/native.go).

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

func show(name string, v any) {
	b, err := json.Marshal(v)
	var ue *json.UnsupportedValueError
	fmt.Println(name, string(b), errors.As(err, &ue))
	fmt.Println("  ", err)
}

func main() {
	m := map[string]any{"k": 1}
	m["self"] = m
	show("map", m)

	s := []any{1, nil}
	s[1] = s
	show("slice", s)

	m2 := map[string]any{}
	m2["s"] = []any{"x", m2}
	show("map through a slice", m2)

	s2 := []any{nil}
	s2[0] = map[string]any{"s": s2}
	show("slice through a map", s2)

	show("in an outer map", map[string]any{"m": m})

	// A map or a slice used twice but not inside itself is not a cycle.
	inner := map[string]any{"a": 1}
	shared := []any{inner, inner, []any{inner}}
	show("shared map", shared)

	// Native code that only moves the elements of a slice that contains
	// itself: the slice is copied back after the call.
	s3 := []any{"b", nil, "a"}
	s3[1] = s3
	sort.SliceStable(s3, func(i, j int) bool {
		x, _ := s3[i].(string)
		y, _ := s3[j].(string)
		return x != "" && y != "" && x < y
	})
	fmt.Println(len(s3), s3[0], s3[2])
}
