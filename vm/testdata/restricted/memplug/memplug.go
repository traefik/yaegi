// Package memplug allocates memory in the ways that Options.MaxMemory
// counts (memquota.go): it keeps large buffers in a global until the quota
// refuses one, and it makes and drops buffers much larger in total than the
// quota. Each route recovers the panic and writes it.
package memplug

import (
	"context"
	"fmt"
	"net/http"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type handler struct{}

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return handler{}, nil
}

var (
	kept  [][]byte
	ints  []int
	str   string
	sink  []byte
	cells [][4096]int
)

// try runs f and returns its panic, as text ("ok" without a panic).
func try(f func()) (s string) {
	defer func() {
		if r := recover(); r != nil {
			s = fmt.Sprint(r)
		}
	}()
	f()
	return "ok"
}

func (handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/keep":
		// 256 KiB buffers kept in a global, until a refusal (or 512 MiB).
		res := try(func() {
			for len(kept) < 2048 {
				kept = append(kept, make([]byte, 256<<10))
			}
		})
		fmt.Fprint(w, res, " ", len(kept))
	case "/ints":
		// A slice of ints that append grows (24 bytes per element in the VM).
		res := try(func() {
			for len(ints) < 1<<24 {
				ints = append(ints, len(ints))
			}
		})
		fmt.Fprint(w, res)
	case "/concat":
		// A string that doubles at each round.
		res := try(func() {
			str = "0123456789abcdef"
			for len(str) < 1<<29 {
				str = str + str
			}
		})
		fmt.Fprint(w, res)
	case "/arrays":
		// Large arrays: new and append of [4096]int elements.
		res := try(func() {
			for len(cells) < 4096 {
				cells = append(cells, *new([4096]int))
			}
		})
		fmt.Fprint(w, res)
	case "/reset":
		kept, ints, str, cells = nil, nil, "", nil
	case "/churn":
		// 2 GiB in 1 MiB buffers, one alive at a time.
		res := try(func() {
			for i := 0; i < 2048; i++ {
				sink = make([]byte, 1<<20)
				sink[i] = 1
			}
		})
		fmt.Fprint(w, res)
	}
}
