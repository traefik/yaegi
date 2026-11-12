//go:build !go1.27

package selfaotplug

import aotjson "github.com/traefik/yaegi/v2/aot/go1_26/encoding/json"

func Run() {
	_, _ = aotjson.Marshal(1)
}
