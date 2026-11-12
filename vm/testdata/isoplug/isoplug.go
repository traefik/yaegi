// Package isoplug keeps state in package variables, set by init functions
// in dependency order. Two loads of the plugin must not share this state,
// and two handlers of one load must share it.
package isoplug

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

var (
	order []string
	mu    sync.Mutex
	hits  = initHits()
	names = map[string]int{}
)

func initHits() int {
	order = append(order, "var")
	return 100
}

func init() { order = append(order, "init1") }
func init() { order = append(order, "init2") }

type Config struct{ Name string }

func CreateConfig() *Config { return &Config{} }

func New(_ context.Context, _ http.Handler, cfg *Config, name string) (http.Handler, error) {
	mu.Lock()
	names[name]++
	mu.Unlock()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		n := hits
		handlers := len(names)
		mu.Unlock()
		fmt.Fprintf(w, "%s %d %d %s", name, n, handlers, strings.Join(order, ","))
	}), nil
}
