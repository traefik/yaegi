package vm

import (
	"runtime"

	"golang.org/x/tools/go/packages"

	"github.com/traefik/yaegi/v2/stdtypes"
)

// loadHooks replace, for one load, what a test cannot change in the
// process: the Go version of the host, whether stdtypes has data for the
// platform, the go list loader, and a function called at the end of each
// load stage. They are a field of the Options (and RunOptions) of the load,
// not package variables, so a test that sets them does not change the loads
// of the other tests that run at the same time. A nil field (always, out of
// the tests) is the real thing.
type loadHooks struct {
	// goVersion is runtime.Version.
	goVersion func() string
	// stdtypesAvailable is stdtypes.Available.
	stdtypesAvailable func() bool
	// packagesLoad is packages.Load.
	packagesLoad func(*packages.Config, ...string) ([]*packages.Package, error)
	// stage is called at the end of each load stage: "built" after the SSA
	// build, "checked" after the preflight, "init" when the package init
	// returns or stops, "recovered" when the load has recovered a panic
	// (while its deferred call runs). Tests use it to cancel the load at an
	// exact point, to see that a cancelled init stopped, or to run the GC at
	// an exact point.
	stage func(stage string)
}

func (h *loadHooks) hostGoVersion() string {
	if h != nil && h.goVersion != nil {
		return h.goVersion()
	}
	return runtime.Version()
}

func (h *loadHooks) haveStdTypes() bool {
	if h != nil && h.stdtypesAvailable != nil {
		return h.stdtypesAvailable()
	}
	return stdtypes.Available()
}

func (h *loadHooks) loadPackages(cfg *packages.Config, patterns ...string) ([]*packages.Package, error) {
	if h != nil && h.packagesLoad != nil {
		return h.packagesLoad(cfg, patterns...)
	}
	return packages.Load(cfg, patterns...)
}

func (h *loadHooks) loadStage(stage string) {
	if h != nil && h.stage != nil {
		h.stage(stage)
	}
}
