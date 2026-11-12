// Package stdsrc is the one list of the standard library packages that the
// VM runs from source, shared by the VM (package vm) and the generator of
// the embedded standard library (cmd/stdexport), which embeds their
// sources (stdtypes.Universe.Source). Like internal/gorelease, it must not
// import the generated packages.
package stdsrc

// Generic are the standard library packages whose generic functions
// programs call: their instances have no native counterpart, so the VM
// interprets them, which needs the package sources.
var Generic = []string{"cmp", "iter", "slices", "maps"}

// Interpreted are the standard library packages that the VM always
// interprets: containers that store interpreted values (as a native copy,
// an element's Value field would come back without its interpreted type,
// and the copy-out after the call would replace interface fields with
// native copies).
var Interpreted = []string{"container/list", "container/ring"}

// All returns Generic and Interpreted: the packages whose sources the
// toolchain-free loader needs.
func All() []string {
	return append(append([]string{}, Generic...), Interpreted...)
}

// Set returns the paths as a set.
func Set(paths []string) map[string]bool {
	out := make(map[string]bool, len(paths))
	for _, p := range paths {
		out[p] = true
	}
	return out
}
