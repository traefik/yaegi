// Package inner is a subpackage of a plugin module: it has no native
// registry entry and must be interpreted.
package inner

import (
	"maps"
	"slices"
	"strings"
)

// Tag is a package variable initialized by the package init.
var Tag = strings.ToUpper("inner")

// Header builds the value of the header set by the plugin.
func Header(name string) string { return Tag + ":" + name }

// Generics of the standard library: instances interpreted from the embedded
// sources of slices and maps (iter.Seq values cross from one to the other).
func Generics(list string, allowed map[string]bool) string {
	parts := slices.Collect(slices.Values(strings.Split(list, ",")))
	keys := slices.Sorted(maps.Keys(allowed))
	var kept []string
	for _, p := range parts {
		if slices.Contains(keys, p) {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, "+") + "/" + strings.Join(keys, "+")
}
