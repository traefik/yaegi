package main

// Writes through slices that come from native memory (http.Header,
// url.Values, struct fields of native types) must reach the native array,
// as in Go. Plugins that rewrite headers depend on it.

import (
	"crypto/tls"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"regexp"
	"runtime/metrics"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"
)

type holder struct {
	Vals []string
	N    int
}

type names []string

func (n names) First() string { return n[0] }

func requestHeaders() {
	req := httptest.NewRequest("GET", "http://example.com/?k=v1&k=v2", nil)
	req.Header.Add("X-Foo", "foo-1")
	req.Header.Add("X-Foo", "foo-2")

	// index store through the map value
	req.Header["X-Foo"][0] = "bar-1"
	fmt.Println(req.Header["X-Foo"])

	// the usual rewrite loop
	for _, vs := range req.Header {
		for i := range vs {
			vs[i] = strings.ReplaceAll(vs[i], "foo", "bar")
		}
	}
	fmt.Println(req.Header["X-Foo"], req.Header.Get("X-Foo"))

	// through a method result
	req.Header.Values("X-Foo")[1] = "baz-2"
	fmt.Println(req.Header.Values("X-Foo"))

	// through a pointer to an element
	p := &req.Header["X-Foo"][0]
	*p = "ptr-1"
	fmt.Println(req.Header["X-Foo"])

	// range with index and value
	for i, v := range req.Header["X-Foo"] {
		req.Header["X-Foo"][i] = v + "!"
	}
	fmt.Println(req.Header["X-Foo"])

	// the query
	q := req.URL.Query()
	q["k"][1] = "w2"
	req.URL.RawQuery = q.Encode()
	fmt.Println(req.URL.RawQuery)

	// the form
	if err := req.ParseForm(); err != nil {
		fmt.Println(err)
	}
	req.Form["k"][0] = "form"
	fmt.Println(req.Form["k"], req.FormValue("k"))
}

func responseHeaders() {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Set-Cookie", "a=1")
		w.Header().Add("Set-Cookie", "b=2")
		for k, vs := range w.Header() {
			if k == "Set-Cookie" {
				for i := range vs {
					vs[i] += "; Secure"
				}
			}
		}
		w.WriteHeader(204)
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	fmt.Println(rec.Code, rec.Result().Header["Set-Cookie"])

	resp := &http.Response{Header: http.Header{"Location": {"http://a/"}}}
	resp.Header["Location"][0] = "http://b/"
	fmt.Println(resp.Header.Get("Location"))
}

func pluginHeader() {
	h := http.Header{}
	h.Add("A", "x")
	h["A"][0] = "y"
	fmt.Println(h)

	v := url.Values{}
	v.Add("k", "v")
	v["k"][0] = "w"
	fmt.Println(v.Encode())

	cfg := &tls.Config{NextProtos: []string{"h2", "http/1.1"}}
	cfg.NextProtos[0] = "h3"
	np := cfg.NextProtos
	cfg.NextProtos = []string{"other"} // np keeps the old slice
	fmt.Println(np, cfg.NextProtos)
}

func aliasing() {
	h := http.Header{}
	h.Add("A", "a")
	h.Add("A", "b")
	h.Add("A", "c") // native appends: capacity 4
	a := h["A"]
	fmt.Println(len(a), cap(a))

	// append within the capacity writes into the shared array
	b := append(a, "d")
	c := h["A"][:4]
	fmt.Println(c, len(b), cap(b), len(h["A"]))
	b[0] = "b0"
	fmt.Println(h["A"][0])

	// append beyond the capacity makes a new array
	d := append(h["A"][:4], "e")
	d[0] = "d0"
	fmt.Println(h["A"][0], d[0], len(d), cap(d))

	// sub-slices
	s := h["A"][1:2]
	s[0] = "s1"
	fmt.Println(h["A"])
	s = s[:cap(s)]
	fmt.Println(len(s), cap(s), s)

	// copy, both ways
	h["B"] = []string{"1", "2", "3"}
	n := copy(h["B"], []string{"x", "y"})
	fmt.Println(n, h["B"])
	n = copy(h["B"][1:], h["B"])
	fmt.Println(n, h["B"])
	local := make([]string, 2)
	n = copy(local, h["B"])
	local[0] = "local"
	fmt.Println(n, local, h["B"])

	// append the other way
	l2 := append([]string{"p"}, h["B"]...)
	l2[1] = "changed"
	fmt.Println(l2, h["B"])
	l3 := append([]string(nil), h["B"]...)
	l3[0] = "l3"
	fmt.Println(l3, h["B"], len(l3), cap(l3))
	l4 := append(h["B"][:0:0], h["B"]...)
	l4[0] = "l4"
	fmt.Println(l4, h["B"])
	var none []string
	l5 := append(none, h["B"][:0]...)
	fmt.Println(l5 == nil)

	// stored in an interpreted struct
	st := holder{Vals: h["B"], N: 1}
	st.Vals[2] = "struct"
	fmt.Println(h["B"])
	st2 := st
	st2.Vals[0] = "copy"
	fmt.Println(h["B"], st.Vals)

	// in an interface
	var x any = h["B"]
	x.([]string)[1] = "iface"
	fmt.Println(h["B"])

	// comparisons with nil
	fmt.Println(h["missing"] == nil, h["B"] == nil, h["B"][:0] == nil, h["B"][:0:0] == nil)

	// passed back to native code
	sort.Strings(h["B"])
	fmt.Println(h["B"], strings.Join(h["B"], ","))
	sort.Slice(h["B"], func(i, j int) bool { return h["B"][i] > h["B"][j] })
	fmt.Println(h["B"])
	sort.StringSlice(h["B"]).Sort()
	fmt.Println(h["B"])
	h2 := http.Header{"C": h["B"]}
	h2["C"][0] = "via h2"
	fmt.Println(h["B"])

	// named slice type of the program
	nm := names(h["B"])
	nm[1] = "named"
	fmt.Println(nm.First(), h["B"], strings.Join(nm, "+"))

	// generic code of package slices (interpreted)
	fmt.Println(slices.Contains(h["B"], "named"), slices.Index(h["B"], "named"))
	slices.Sort(h["B"])
	fmt.Println(h["B"])
	cl := slices.Clone(h["B"])
	cl[0] = "clone"
	fmt.Println(cl, h["B"])
	slices.Reverse(h["B"])
	fmt.Println(h["B"])
	h["B"] = slices.Insert(h["B"], 1, "ins")
	fmt.Println(h["B"])
	h["B"] = slices.Delete(h["B"], 0, 1)
	fmt.Println(h["B"])

	// conversion to an array and to an array pointer
	ap := (*[2]string)(h["B"])
	ap[0] = "arrptr"
	arr := [2]string(h["B"])
	arr[1] = "arr"
	fmt.Println(h["B"], arr)

	// clear
	clear(h["B"][1:])
	fmt.Printf("%q\n", h["B"])

	// json and fmt of a struct that holds a view
	out, err := json.Marshal(st)
	fmt.Println(string(out), err)
	fmt.Printf("%v %+v\n", st, st)
}

func otherScalars() {
	// a []int from native code
	re := regexp.MustCompile(`b+`)
	loc := re.FindStringIndex("aabbbcc")
	loc[0]++
	fmt.Println(loc, len(loc), cap(loc))
	all := re.FindAllStringIndex("abbab", -1)
	all[1][0] = 100
	fmt.Println(all)

	// a []rune and a []float64
	rs := []rune(strings.ToUpper("héllo"))
	rs[0] = 'J'
	fmt.Println(string(rs), utf8.RuneCountInString(string(rs)))
	fs := strings.Fields(" a b  c ")
	fs[2] = "z"
	fmt.Println(fs, len(fs))
}

// decodeIntoViews decodes into variables and fields that hold a view. The
// decoder changes the header of the variable (length, capacity, array) and
// writes the elements: the elements it writes within the old length reach
// the native array, a new array does not, as in Go.
func decodeIntoViews() {
	// strings: shrink, same length, grow past the capacity
	h := http.Header{"A": {"b", "a"}}
	vs := h["A"]
	err := json.Unmarshal([]byte(`["x"]`), &vs)
	fmt.Println(vs, len(vs), cap(vs), h["A"], err)
	vs = h["A"]
	err = json.Unmarshal([]byte(`["y","z"]`), &vs)
	fmt.Println(vs, len(vs), cap(vs), h["A"], err)
	q := url.Values{"k": {"1", "2", "3"}}
	n := q["k"]
	err = json.Unmarshal([]byte(`["p","q","r","s","t"]`), &n)
	n[0] = "new array"
	fmt.Println(n, len(n), q["k"], err)
	n = q["k"]
	err = json.Unmarshal([]byte(`[]`), &n)
	fmt.Println(n, n == nil, len(n), q["k"], err)
	n = q["k"]
	err = json.Unmarshal([]byte(`null`), &n)
	fmt.Println(n, n == nil, q["k"], err)

	// a struct field that holds a view
	t := holder{Vals: q["k"]}
	err = json.Unmarshal([]byte(`{"Vals":["f"],"N":2}`), &t)
	fmt.Println(t, q["k"], err)
	t = holder{Vals: q["k"]}
	err = json.Unmarshal([]byte(`{"Vals":["f1","f2","f3","f4"]}`), &t)
	fmt.Println(t, q["k"], err)

	// ints: shrink, same length, grow past the capacity
	re := regexp.MustCompile(`b+`)
	loc := re.FindStringIndex("aabbbcc")
	is := loc
	err = json.Unmarshal([]byte(`[7]`), &is)
	fmt.Println(is, len(is), loc, err)
	is = loc
	err = json.Unmarshal([]byte(`[8,9]`), &is)
	fmt.Println(is, loc, err)
	is = loc
	err = json.Unmarshal([]byte(`[1,2,3,4,5]`), &is)
	fmt.Println(is, loc, err)

	// floats and uints (fields of a native struct): shrink, same length,
	// grow past the capacity
	hist := &metrics.Float64Histogram{Counts: []uint64{1, 2}, Buckets: []float64{0.5, 1.5, 2.5}}
	fl := hist.Buckets
	err = json.Unmarshal([]byte(`[9.25]`), &fl)
	fmt.Println(fl, len(fl), hist.Buckets, err)
	fl = hist.Buckets
	err = json.Unmarshal([]byte(`[3.5,4.5,5.5]`), &fl)
	fmt.Println(fl, hist.Buckets, err)
	fl = hist.Buckets
	err = json.Unmarshal([]byte(`[1,2,3,4,5,6,7]`), &fl)
	fmt.Println(fl, hist.Buckets, err)
	us := hist.Counts
	err = json.Unmarshal([]byte(`[10,20,30]`), &us)
	fmt.Println(us, hist.Counts, err)
	us = hist.Counts
	err = json.Unmarshal([]byte(`[40]`), &us)
	fmt.Println(us, hist.Counts, err)

	// xml appends to the slice
	type xv struct {
		V []string `xml:"v"`
	}
	x := xv{V: q["k"][:1]}
	err = xml.Unmarshal([]byte("<x><v>m</v><v>n</v></x>"), &x)
	fmt.Println(x.V, q["k"], err)

	// the reflect of the program on a variable that holds a view
	vs = h["A"]
	rv := reflect.ValueOf(&vs).Elem()
	rv.Index(0).SetString("r0")
	fmt.Println(vs, h["A"])
	rv.SetLen(1)
	fmt.Println(vs, len(vs), cap(vs), h["A"])
	rv.Set(reflect.Append(rv, reflect.ValueOf("r1")))
	fmt.Println(vs, h["A"])
	rv.Set(reflect.Append(rv, reflect.ValueOf("r2")))
	vs[0] = "past cap"
	fmt.Println(vs, h["A"])
	vs = h["A"]
	rv = reflect.ValueOf(&vs).Elem()
	rv.Grow(10)
	vs[0] = "grown"
	fmt.Println(vs, cap(vs) >= 12, h["A"])
	vs = h["A"]
	rv = reflect.ValueOf(&vs).Elem()
	fmt.Println(reflect.Copy(rv, reflect.ValueOf([]string{"c0"})), vs, h["A"])
	rv.SetLen(1)
	rv.SetCap(1)
	vs = append(vs, "w") // past the capacity: a new array
	vs[0] = "c1"
	fmt.Println(vs, cap(vs) > 1, h["A"])
	rv.SetZero()
	fmt.Println(vs == nil, h["A"])
	vs = h["A"]
	reflect.ValueOf(&vs).Elem().Set(reflect.ValueOf(vs[:1]))
	vs[0] = "set"
	fmt.Println(vs, h["A"])
}

func main() {
	requestHeaders()
	responseHeaders()
	pluginHeader()
	aliasing()
	otherScalars()
	decodeIntoViews()
}
