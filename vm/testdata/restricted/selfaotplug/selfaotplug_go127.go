//go:build go1.27

// Package selfaotplug imports an AOT copy of yaegi v2 (aot/go1_NN/encoding/json,
// one file per Go release of the window) by its own import path. The module
// of the test is yaegi v2 itself, so the loader finds its sources. A plugin must
// not run the AOT copies (or the vreflect and vm packages they import) from
// source: it may not load.
package selfaotplug

import aotjson "github.com/traefik/yaegi/v2/aot/go1_27/encoding/json"

func Run() {
	_, _ = aotjson.Marshal(1)
}
