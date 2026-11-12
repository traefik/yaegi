package vm

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// nopWriter is a ResponseWriter that drops the response.
type nopWriter struct{ h http.Header }

func (w nopWriter) Header() http.Header         { return w.h }
func (w nopWriter) Write(b []byte) (int, error) { return len(b), nil }
func (w nopWriter) WriteHeader(int)             {}

// BenchmarkServeBudget measures the cost of a trivial request of a plugin,
// without a budget and with one (an account per request, budget.go), on one
// goroutine and on all of them.
func BenchmarkServeBudget(b *testing.B) {
	for _, bc := range []struct {
		name   string
		budget int64
	}{{"none", 0}, {"budget", 1e8}} {
		_, h := loadBudgetPlug(b, Options{Budget: bc.budget}, "bench")
		req := httptest.NewRequest("GET", "/iters", nil)
		b.Run(bc.name+"/serial", func(b *testing.B) {
			w := nopWriter{http.Header{}}
			b.ReportAllocs()
			for b.Loop() {
				h.ServeHTTP(w, req)
			}
		})
		b.Run(bc.name+"/parallel", func(b *testing.B) {
			b.ReportAllocs()
			b.RunParallel(func(pb *testing.PB) {
				w := nopWriter{http.Header{}}
				for pb.Next() {
					h.ServeHTTP(w, req)
				}
			})
		})
	}
}
