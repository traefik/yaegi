// Package envsysplug reads its environment with package syscall (a plugin
// with useUnsafe).
package envsysplug

import (
	"strings"
	"syscall"
)

// Get returns syscall.Getenv(k).
func Get(k string) (string, bool) { return syscall.Getenv(k) }

// Environ returns the entries of syscall.Environ() whose name starts with
// prefix.
func Environ(prefix string) []string {
	var out []string
	for _, kv := range syscall.Environ() {
		if strings.HasPrefix(kv, prefix) {
			out = append(out, kv)
		}
	}
	return out
}
