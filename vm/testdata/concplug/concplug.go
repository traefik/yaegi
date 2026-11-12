// Package concplug is a plugin whose handler uses the concurrency features
// of Go on each request, while many requests run at the same time: a fan-out
// to worker goroutines over channels, a select with a timeout, a shared
// counter under a mutex, an atomic counter, a sync.Map cache, a generic
// cache type, closures, a deferred recover, and json. The body of each
// request only depends on its id, so the test can check every response.
package concplug

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

type Config struct {
	Workers int `json:"workers"`
}

func CreateConfig() *Config { return &Config{Workers: 4} }

type Cache[K comparable, V any] struct {
	mu sync.RWMutex
	m  map[K]V
}

func (c *Cache[K, V]) GetOrSet(k K, f func() V) V {
	c.mu.RLock()
	v, ok := c.m[k]
	c.mu.RUnlock()
	if ok {
		return v
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if v, ok := c.m[k]; ok {
		return v
	}
	v = f()
	c.m[k] = v
	return v
}

// Score has no MarshalJSON: in VM mode, the methods of the types of inner
// fields are not seen by the native json (TODO.md, section 1).
type Score int

type result struct {
	ID     int      `json:"id"`
	Sum    int      `json:"sum"`
	Score  Score    `json:"score"`
	Words  []string `json:"words"`
	Cached string   `json:"cached"`
	Err    string   `json:"err,omitempty"`
}

var errOdd = errors.New("odd id")

type state struct {
	mu      sync.Mutex
	perPath map[int]int
	total   atomic.Int64
	seen    sync.Map
	cache   *Cache[int, string]
}

func New(_ context.Context, _ http.Handler, cfg *Config, _ string) (http.Handler, error) {
	st := &state{perPath: map[int]int{}, cache: &Cache[int, string]{m: map[int]string{}}}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/stats" {
			st.mu.Lock()
			n := len(st.perPath)
			st.mu.Unlock()
			keys := 0
			st.seen.Range(func(_, _ any) bool { keys++; return true })
			fmt.Fprintf(w, "total=%d paths=%d seen=%d", st.total.Load(), n, keys)
			return
		}
		id, err := strconv.Atoi(r.URL.Path[1:])
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		st.total.Add(1)
		st.mu.Lock()
		st.perPath[id%50]++
		st.mu.Unlock()
		st.seen.LoadOrStore(id%50, struct{}{})

		// Fan out: each worker squares the numbers it receives.
		jobs := make(chan int)
		out := make(chan int, cfg.Workers)
		var wg sync.WaitGroup
		for range cfg.Workers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := range jobs {
					out <- j * j
				}
			}()
		}
		go func() {
			for i := range 10 {
				jobs <- id + i
			}
			close(jobs)
		}()
		go func() { wg.Wait(); close(out) }()
		sum := 0
		for v := range out {
			sum += v
		}

		// Closures over per-iteration variables, sorted after a race
		// between goroutines.
		words := make([]string, 0, 3)
		var wmu sync.Mutex
		var wg2 sync.WaitGroup
		for i, s := range []string{"c", "a", "b"} {
			wg2.Add(1)
			go func() {
				defer wg2.Done()
				wmu.Lock()
				words = append(words, s+strconv.Itoa(id+i))
				wmu.Unlock()
			}()
		}
		wg2.Wait()
		sort.Strings(words)

		// A select that never times out, and one that always does.
		done := make(chan struct{})
		go func() { close(done) }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			http.Error(w, "timeout", 500)
			return
		}
		select {
		case <-make(chan int):
		case <-time.After(time.Microsecond):
		}

		res := result{
			ID:     id,
			Sum:    sum,
			Score:  Score(id % 7),
			Words:  words,
			Cached: st.cache.GetOrSet(id%10, func() string { return "v" + strconv.Itoa(id%10) }),
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					res.Err = fmt.Sprint(r)
				}
			}()
			if id%2 == 1 {
				panic(fmt.Errorf("checked: %w", errOdd))
			}
		}()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(res)
	}), nil
}
