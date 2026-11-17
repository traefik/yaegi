// Package shims has the shims of yaegi v2 (vm.Shim): source files of
// libraries that read the internals of the Go runtime, rewritten in plain Go
// that the VM runs. host.Options sets All.
//
// A shim replaces a file only when its content has one of the sums of the
// shim, so each shim is for given versions of a library. To add a version,
// check that the file did not change in a way the shim does not follow, then
// add its sum (sha256sum of the file).
package shims

import (
	_ "embed"

	"github.com/traefik/yaegi/v2/vm"
)

//go:embed dolthub_maphash_runtime.go.txt
var dolthubMaphashRuntime []byte

// DolthubMaphash replaces runtime.go of github.com/dolthub/maphash (used by
// github.com/maypok86/otter v1): it reads the hash function of the runtime
// type of a map through the type word of an interface. The shim hashes the
// key with hash/maphash.WriteComparable.
var DolthubMaphash = vm.Shim{
	File: "runtime.go",
	SHA256: []string{
		"da75adbd3a2dbac156c7dc2bf8e553841b2471604918819a42527e38eef4b128", // v0.1.0
	},
	Src: dolthubMaphashRuntime,
	Why: "github.com/dolthub/maphash reads the hash function of a runtime map type",
}

// All returns the shims of yaegi v2.
func All() []vm.Shim {
	return []vm.Shim{DolthubMaphash}
}
