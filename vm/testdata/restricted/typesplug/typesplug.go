// Package typesplug changes the go/types objects of the host, which the VM
// uses to type-check the plugins: a new name in the universe would be
// declared for every plugin loaded later. It may not load.
package typesplug

import (
	"go/constant"
	"go/token"
	"go/types"
)

func Run() {
	types.Universe.Insert(types.NewConst(token.NoPos, nil, "rv64pwned", types.Typ[types.Int], constant.MakeInt64(42)))
	types.Unsafe.SetName("evil")
}
