// A Context of the program given to native fcgi.ProcessEnv, which asserts
// its value to map[string]string. A value of another type gives nil, and
// the program still gets its value back from the request's context.
//
// Known limit, not tested here: with -aot (and OpaqueAny) a map from a
// Context of the program stays an opaque handle, to keep aliasing as in Go
// (vm.TestContextValueAliasOpaque), so ProcessEnv gives nil for it
// (testdata/boundary/known.tsv, ProcessEnv aot).
package main

import (
	"context"
	"fmt"
	"net/http/fcgi"
	"net/http/httptest"
)

type envCtx struct {
	context.Context
	v any
}

func (c envCtx) Value(key any) any { return c.v }

func main() {
	r := httptest.NewRequest("GET", "/plain", nil)
	r = r.WithContext(envCtx{context.Background(), []string{"x"}})
	fmt.Println(fcgi.ProcessEnv(r) == nil, r.Context().Value("k"))
	r = r.WithContext(envCtx{context.Background(), struct{ K, V string }{"K", "v"}})
	fmt.Println(fcgi.ProcessEnv(r) == nil, r.Context().Value("k"))
	r = r.WithContext(envCtx{context.Background(), [2]int{1, 2}})
	fmt.Println(fcgi.ProcessEnv(r) == nil, r.Context().Value("k"))
}
