// Command loadtime measures how long loading a plugin takes with the
// toolchain-free loader of vm.Load: the first load in the process (which
// also decodes the embedded standard library types) and a second load of the
// same plugin (the standard library is then shared), split into stages.
//
//	loadtime [-aot] [-n 3] dir import [dir import...]
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"time"

	_ "github.com/traefik/yaegi/v2/aot/all"
	_ "github.com/traefik/yaegi/v2/natsyms/all"
	"github.com/traefik/yaegi/v2/stdtypes"
	"github.com/traefik/yaegi/v2/vm"
)

func main() {
	aot := flag.Bool("aot", false, "AOT mode")
	n := flag.Int("n", 3, "warm loads per plugin (the median is printed)")
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 || len(args)%2 != 0 {
		fmt.Fprintln(os.Stderr, "usage: loadtime [-aot] [-n 3] dir import [dir import...]")
		os.Exit(2)
	}
	t0 := time.Now()
	if _, err := stdtypes.Load(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("stdtypes decode: %v\n", time.Since(t0).Round(time.Millisecond))
	fmt.Printf("%-50s %10s %10s %10s %10s %10s\n", "plugin", "parse+tc", "ssa", "check", "init", "total")
	for i := 0; i < len(args); i += 2 {
		dir, imp := args[i], args[i+1]
		var totals []time.Duration
		var last *vm.Plugin
		for k := 0; k < *n; k++ {
			t := time.Now()
			p, err := vm.Load(vm.Options{Dir: dir, AOT: *aot}, imp)
			if err != nil {
				fmt.Printf("%-50s error: %v\n", imp, err)
				break
			}
			totals = append(totals, time.Since(t))
			last = p
		}
		if last == nil {
			continue
		}
		sort.Slice(totals, func(a, b int) bool { return totals[a] < totals[b] })
		ms := func(d time.Duration) string { return fmt.Sprintf("%.1fms", float64(d.Microseconds())/1000) }
		fmt.Printf("%-50s %10s %10s %10s %10s %10s\n", imp, ms(last.LoadTime), ms(last.BuildTime), ms(last.CheckTime), ms(last.InitTime), ms(totals[len(totals)/2]))
	}
}
