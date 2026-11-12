// Package hostpkgplug imports packages that act on the host process: go/build
// and go/importer run the go command, net/http/pprof serves the state of
// the host on http.DefaultServeMux. It imports expvar too, which a
// restricted plugin may import (a table of its own) when the host imports
// stdsyms/sideeffect (vm/sideeffecttest).
package hostpkgplug

import (
	"expvar"
	"go/build"
	"go/importer"
	"go/token"
	_ "net/http/pprof"
)

func Run() {
	build.Default.Import("example.com/x", ".", 0)
	importer.ForCompiler(token.NewFileSet(), "gc", nil)
	expvar.NewInt("hits")
}
