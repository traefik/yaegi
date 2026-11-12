// Package envplug reads and changes its environment with every function of
// package os that works on it, by direct calls and by function values.
package envplug

import (
	"os"
	"strings"
)

// atInit is read by the init of the package, as plugins read their settings.
var atInit = os.Getenv("YAEGIENV_A")

// AtInit returns the value of YAEGIENV_A when the package was initialized.
func AtInit() string { return atInit }

// Get returns os.Getenv(k).
func Get(k string) string { return os.Getenv(k) }

// GetValue returns os.Getenv(k) through a function value.
func GetValue(k string) string {
	f := os.Getenv
	return f(k)
}

// Lookup returns os.LookupEnv(k).
func Lookup(k string) (string, bool) { return os.LookupEnv(k) }

// Environ returns the entries of os.Environ() whose name starts with prefix,
// and the number of all the entries.
func Environ(prefix string) ([]string, int) {
	all := os.Environ()
	var out []string
	for _, kv := range all {
		if strings.HasPrefix(kv, prefix) {
			out = append(out, kv)
		}
	}
	return out, len(all)
}

// Expand returns os.ExpandEnv(s).
func Expand(s string) string { return os.ExpandEnv(s) }

// Set calls os.Setenv(k, v), through a function value.
func Set(k, v string) error {
	f := os.Setenv
	return f(k, v)
}

// Unset calls os.Unsetenv(k).
func Unset(k string) error { return os.Unsetenv(k) }

// Clear calls os.Clearenv().
func Clear() { os.Clearenv() }
