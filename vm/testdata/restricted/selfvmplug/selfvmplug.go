// Package selfvmplug imports package vm, the engine of yaegi v2. The module
// of the test is yaegi v2 itself, so the loader finds the sources of vm. A plugin
// must not run the engine from source: it may not load.
package selfvmplug

import "github.com/traefik/yaegi/v2/vm"

func Run() {
	_, _ = vm.Load(vm.Options{Unrestricted: true, RealProcess: true}, "x")
}
