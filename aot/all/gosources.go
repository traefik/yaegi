package all

import (
	_ "embed"

	"github.com/traefik/yaegi/v2/vm"
)

// gosources.txt is the manifest of the GOROOT sources of the copied
// standard packages, for each Go version (internal/gosources): with it, a
// host built with a newer patch release than the AOT copies keeps the
// copies when that patch release did not change their sources.
//
//go:embed gosources.txt
var goSources []byte

func init() { vm.RegisterAOTGoSources(goSources) }
