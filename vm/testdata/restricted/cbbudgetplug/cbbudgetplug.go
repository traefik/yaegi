// Package cbbudgetplug spends its budget inside callbacks from native code.
package cbbudgetplug

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

// spin never returns.
type spin struct{}

func (spin) String() string {
	n := 0
	for {
		if n++; n < 0 {
			break
		}
	}
	return "never"
}

// boom panics in its String method, like an ordinary bug.
type boom struct{}

func (boom) String() string { panic("boom") }

// work runs n backward jumps. The sum is an int64: work(600000) overflows
// a 32-bit int.
func work(n int) int64 {
	var s int64
	for i := 0; i < n; i++ {
		s += int64(i)
	}
	return s
}

func New(_ context.Context, next http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/fmt":
			// fmt recovers the panics of String methods: the budget
			// error must not be turned into "%!v(PANIC=...)".
			s := fmt.Sprint(spin{})
			fmt.Fprint(w, s)
		case "/map":
			// Each call of the mapping function stays far below the
			// budget, but all of them together go past it.
			s := strings.Map(func(c rune) rune { work(1000); return c }, strings.Repeat("a", 10000))
			fmt.Fprint(w, len(s))
		case "/defer":
			// The deferred fmt.Sprint runs first and fmt recovers the
			// budget error of the String method: the other deferred call
			// must not run.
			defer fmt.Fprint(w, "after")
			defer fmt.Sprint(spin{})
		case "/chain":
			// More than half of the budget, then the next handler. When
			// the same plugin comes twice in a chain, each call has its
			// own budget.
			fmt.Fprint(w, work(600000)%10)
			next.ServeHTTP(w, r)
		case "/gofmt":
			// A goroutine of the program that runs a native function
			// has a budget too, also in the callbacks of that function.
			go fmt.Sprint(spin{})
		case "/gomap":
			go strings.Map(func(c rune) rune { work(1000); return c }, strings.Repeat("a", 10000))
		case "/small":
			s := strings.Map(func(c rune) rune { work(10); return c }, strings.Repeat("a", 100))
			fmt.Fprint(w, len(s), " ", boom{})
		}
	}), nil
}
