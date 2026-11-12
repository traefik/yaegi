// Decoding into a pointer to an interface that holds a non nil pointer.
// encoding/json, xml and gob keep the dynamic type and
// fill the pointed value; native code gets a copy of the pointed value, which
// must be written back into the interpreted cell, so that every alias sees it.
package main

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/url"
	"strings"
)

type authResponse struct {
	Token string `json:"token" xml:"token"`
	N     int    `json:"n" xml:"n"`
}

type Payload interface{}

type Labeler interface{ Label() string }

type named struct{ Name string }

func (n *named) Label() string { return "named:" + n.Name }

type holder struct {
	V any
}

type node struct {
	Name string
	Next any
}

func show(label string, v any) {
	fmt.Printf("%s: %T %+v\n", label, v, v)
}

func helper(data []byte, out any) error {
	return json.Unmarshal(data, &out)
}

func main() {
	// The case of the issue.
	{
		var value any = &authResponse{}
		err := json.Unmarshal([]byte(`{"token":"tok-123","n":4}`), &value)
		show("struct", value)
		fmt.Println(err)
	}
	// The pointed value is aliased: the alias sees the decoded value.
	{
		a := &authResponse{N: 1}
		var value any = a
		err := json.Unmarshal([]byte(`{"token":"tok"}`), &value)
		fmt.Println("alias:", value.(*authResponse) == a, *a, err)
	}
	// A helper that takes any and decodes into &out (traefik-umami-feeder).
	{
		var r authResponse
		err := helper([]byte(`{"token":"helper"}`), &r)
		fmt.Println("helper:", r, err)
	}
	// Pointers to a map, a slice, an int, a string.
	{
		m := map[string]int{"a": 1}
		var value any = &m
		err := json.Unmarshal([]byte(`{"b":2}`), &value)
		m["c"] = 3
		fmt.Println("map:", *value.(*map[string]int), value.(*map[string]int) == &m, err)
	}
	{
		s := []int{9}
		var value any = &s
		err := json.Unmarshal([]byte(`[1,2,3]`), &value)
		fmt.Println("slice:", s, value.(*[]int) == &s, err)
	}
	{
		n := 1
		var value any = &n
		err := json.Unmarshal([]byte(`42`), &value)
		fmt.Println("int:", n, value.(*int) == &n, err)
	}
	{
		str := "old"
		var value any = &str
		err := json.Unmarshal([]byte(`"new"`), &value)
		fmt.Println("string:", str, value.(*string) == &str, err)
	}
	// A pointer to a native named type.
	{
		q := url.Values{"a": {"1"}}
		var value any = &q
		err := json.Unmarshal([]byte(`{"b":["2"]}`), &value)
		fmt.Println("native named:", q.Encode(), value.(*url.Values) == &q, err)
	}
	// A nil pointer in the interface: Go replaces it.
	{
		var value any = (*authResponse)(nil)
		err := json.Unmarshal([]byte(`{"token":"x"}`), &value)
		show("nil pointer", value)
		fmt.Println(err)
	}
	// A non-pointer value in the interface: Go replaces it.
	{
		var value any = authResponse{Token: "old"}
		err := json.Unmarshal([]byte(`{"token":"x"}`), &value)
		show("value", value)
		fmt.Println(err)
	}
	// null: Go sets the interface to nil and keeps the pointed value.
	{
		a := &authResponse{Token: "keep"}
		var value any = a
		err := json.Unmarshal([]byte(`null`), &value)
		fmt.Println("null:", value, *a, err)
	}
	// A named interface, and a pointer type with methods.
	{
		n := &named{}
		var value Payload = n
		err := json.Unmarshal([]byte(`{"Name":"bob"}`), &value)
		fmt.Println("named iface:", value.(*named).Label(), n.Name, err)
	}
	{
		n := &named{}
		var value Labeler = n
		err := json.Unmarshal([]byte(`{"Name":"ann"}`), &value)
		fmt.Println("iface with methods:", value.Label(), n.Name, err)
	}
	// json.Decoder.Decode.
	{
		a := &authResponse{}
		var value any = a
		err := json.NewDecoder(strings.NewReader(`{"token":"dec"}`)).Decode(&value)
		fmt.Println("decoder:", *a, value.(*authResponse) == a, err)
	}
	// A struct field of type any that holds a pointer.
	{
		a := &authResponse{}
		h := holder{V: a}
		err := json.Unmarshal([]byte(`{"V":{"token":"field","n":7}}`), &h)
		fmt.Println("field:", *a, h.V.(*authResponse) == a, err)
	}
	// Nested: a pointer in an interface that points to a struct with an
	// interface that holds another pointer.
	{
		inner := &node{Name: "inner"}
		outer := &node{Next: inner}
		var value any = outer
		err := json.Unmarshal([]byte(`{"Name":"outer","Next":{"Name":"deep"}}`), &value)
		fmt.Println("nested:", outer.Name, inner.Name, outer.Next.(*node) == inner, err)
	}
	// A pointer that points to itself through an interface.
	{
		self := &node{}
		self.Next = self
		var value any = self
		err := json.Unmarshal([]byte(`{"Name":"self","Next":{"Name":"again"}}`), &value)
		fmt.Println("cycle:", self.Name, self.Next.(*node) == self, err)
	}
	// Elements of a slice and values of a map of any.
	{
		a := &authResponse{}
		s := []any{a}
		err := json.Unmarshal([]byte(`[{"token":"elem"}]`), &s)
		fmt.Println("slice elem:", *a, s[0].(*authResponse) == a, err)
	}
	// json decodes into the pointers that are already in a slice or an
	// array, also in a struct element.
	{
		a, b := &authResponse{}, &authResponse{}
		s := []*authResponse{a, b}
		err := json.Unmarshal([]byte(`[{"token":"p0"},{"n":2}]`), &s)
		fmt.Println("pointer elems:", *a, *b, s[0] == a, s[1] == b, err)
	}
	{
		a := &authResponse{}
		arr := [2]any{a, 5}
		err := json.Unmarshal([]byte(`[{"token":"arr"},"x"]`), &arr)
		fmt.Println("array:", *a, arr[0].(*authResponse) == a, arr[1], err)
	}
	{
		a := &authResponse{}
		s := []holder{{V: a}}
		err := json.Unmarshal([]byte(`[{"V":{"token":"in struct"}}]`), &s)
		fmt.Println("struct elem:", *a, s[0].V.(*authResponse) == a, err)
	}
	{
		a := &authResponse{}
		mp := map[string]*authResponse{"k": a}
		err := json.Unmarshal([]byte(`{"k":{"token":"replaced"}}`), &mp)
		fmt.Println("map of pointers:", *a, *mp["k"], mp["k"] == a, err)
	}
	{
		a := &authResponse{}
		mp := map[string]any{"k": a}
		err := json.Unmarshal([]byte(`{"k":{"token":"mapval"}}`), &mp)
		show("map value", mp["k"])
		fmt.Println(*a, err)
	}
	// An error after part of the data: what was decoded stays.
	{
		a := &authResponse{}
		var value any = a
		err := json.Unmarshal([]byte(`{"token":"part","n":"bad"}`), &value)
		fmt.Println("error:", *a, err != nil)
	}
	// xml and gob.
	{
		a := &authResponse{}
		var value any = a
		err := xml.Unmarshal([]byte(`<authResponse><token>xml</token><n>3</n></authResponse>`), &value)
		fmt.Println("xml:", *a, err != nil)
	}
	{
		var buf bytes.Buffer
		if err := gob.NewEncoder(&buf).Encode(authResponse{Token: "gob", N: 5}); err != nil {
			panic(err)
		}
		a := &authResponse{}
		var value any = a
		err := gob.NewDecoder(&buf).Decode(&value)
		fmt.Println("gob:", *a, err != nil)
	}
}
