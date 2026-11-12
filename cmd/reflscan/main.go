// Command reflscan lists the reflect / internal/reflectlite API used by a set
// of packages (static calls and interface method calls on reflect.Type),
// i.e. what a VM-internal reflect must implement to interpret them.
package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

func main() {
	pkgs, err := packages.Load(&packages.Config{Mode: packages.LoadSyntax}, os.Args[1:]...)
	if err != nil || packages.PrintErrors(pkgs) > 0 {
		panic(err)
	}
	prog, spkgs := ssautil.Packages(pkgs, ssa.InstantiateGenerics)
	for _, p := range spkgs {
		p.Build()
	}
	used := map[string]int{}
	isRefl := func(path string) bool { return path == "reflect" || path == "internal/reflectlite" }
	for fn := range ssautil.AllFunctions(prog) {
		if fn.Pkg == nil || len(fn.Blocks) == 0 || isRefl(fn.Pkg.Pkg.Path()) {
			continue
		}
		for _, b := range fn.Blocks {
			for _, ins := range b.Instrs {
				c, ok := ins.(ssa.CallInstruction)
				if !ok {
					continue
				}
				cc := c.Common()
				if cc.IsInvoke() {
					if t := cc.Value.Type().String(); strings.HasPrefix(t, "reflect.") || strings.HasPrefix(t, "internal/reflectlite.") {
						used["invoke "+t+"."+cc.Method.Name()]++
					}
					continue
				}
				if f := cc.StaticCallee(); f != nil {
					o := f
					if f.Origin() != nil {
						o = f.Origin()
					}
					if o.Pkg != nil && isRefl(o.Pkg.Pkg.Path()) {
						used["call "+o.String()]++
					}
				}
			}
		}
	}
	keys := make([]string, 0, len(used))
	for k := range used {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("%4d %s\n", used[k], k)
	}
	fmt.Println(len(keys), "entry points")
}
