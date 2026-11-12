// Package nativerecplug recurses without end through callbacks that native
// code calls back (strings.Map, sort.Slice, a fmt Stringer, a deferred
// call), in ServeHTTP or in a goroutine. The path of the request chooses
// the way.
package nativerecplug

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

func viaMap(s string) string {
	return strings.Map(func(r rune) rune { viaMap(s); return r }, s)
}

func viaSort(s []int) {
	sort.Slice(s, func(i, j int) bool { viaSort(s); return s[i] < s[j] })
}

type node struct{ n int }

func (n *node) String() string { return fmt.Sprint(&node{n.n + 1}) }

func viaDefer(n int) {
	defer viaDefer(n + 1)
}

// deepThenMap goes n frames deep, then m frames more in a callback of
// strings.Map: finite, but past the depth limit of plugins (100000
// frames) when n+m is larger.
func deepThenMap(n, m int) int {
	if n > 0 {
		return deepThenMap(n-1, m) + 1
	}
	k := 0
	strings.Map(func(r rune) rune { k = down(m); return r }, "x")
	return k
}

func down(m int) int {
	if m == 0 {
		return 0
	}
	return down(m-1) + 1
}

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/map":
			viaMap("ab")
		case "/sort":
			viaSort([]int{2, 1})
		case "/stringer":
			fmt.Fprint(w, &node{})
		case "/defer":
			viaDefer(0)
		case "/deep":
			deepThenMap(99000, 2000)
		case "/go":
			go viaMap("ab")
		}
		fmt.Fprint(w, "survived")
	}), nil
}
