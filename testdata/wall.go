package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
)

// Interpreted interface + interpreted named type with a method.
type Stringer interface{ String() string }

type T int

func (t T) String() string { return "T(" + strconv.Itoa(int(t)) + ")" }

type MyErr struct{ msg string }

func (e MyErr) Error() string { return "myerr: " + e.msg }

type H struct{ prefix string }

func (h H) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Vm", "yes")
	w.WriteHeader(202)
	w.Write([]byte(h.prefix + r.URL.Path))
}

func describe(x any) string {
	switch v := x.(type) {
	case Stringer:
		return "stringer:" + v.String()
	case int:
		return "int:" + strconv.Itoa(v)
	case error:
		return "error:" + v.Error()
	}
	return "other"
}

func main() {
	// 1. Entirely inside the VM: no reflect involved.
	println(describe(T(5)), describe(7), describe(MyErr{"x"}), describe("s"))

	// 2. Interpreted value -> native fmt (Stringer wrapper chosen by method set).
	fmt.Println(T(5), MyErr{"boom"})
	fmt.Printf("v=%v d=%d T=%T\n", T(5), T(5), T(5))

	// 3. Round trip through native code keeps identity.
	var e error = MyErr{"wrapped"}
	werr := fmt.Errorf("ctx: %w", e)
	println(werr.Error())

	// 4. Native net/http calls an interpreted http.Handler (method) and a closure.
	mux := http.NewServeMux()
	mux.Handle("/", H{prefix: "hello "})
	mux.Handle("/f", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(201)
		w.Write([]byte("from closure " + r.Method))
	}))
	for _, path := range []string{"/world", "/f"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		println(rec.Code, rec.Header().Get("X-Vm"), rec.Body.String())
	}

	// 5. Aliasing across the boundary: slices are copied, in-place updates copied back.
	xs := []int{3, 1, 2}
	sort.Ints(xs)
	fmt.Println(xs)
}
