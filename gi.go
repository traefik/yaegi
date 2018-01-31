package main

import (
	"github.com/containous/gi/interp"
)

func main() {
	src := `
package main

func main() {
	println(1)
	for a := 0; a < 10000; a++ {
		if (a & 0x8ff) == 0x800 {
			println(a)
		}
	}
}
`
	root := interp.SrcToAst(src)
	//root.AstDot()
	cfg_entry := root.Child[1].Child[2] // FIXME: entry point should be resolved from 'main' name
	cfg_entry.AstToCfg()
	cfg_entry.OptimCfg()
	//cfg_entry.CfgDot()
	interp.RunCfg(cfg_entry.Start)
}
