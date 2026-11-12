// Command hostimport is a host that imports only package vm. It prints
// what the import added to the global state of the process: the handlers
// of http.DefaultServeMux under /debug/, and the flags of flag.CommandLine.
// TestHostImport wants no output.
package main

import (
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"

	_ "github.com/traefik/yaegi/v2/vm"
)

func main() {
	for _, path := range []string{"/debug/pprof/", "/debug/vars", "/debug/"} {
		if _, pattern := http.DefaultServeMux.Handler(httptest.NewRequest("GET", path, nil)); pattern != "" {
			fmt.Printf("http.DefaultServeMux: %s\n", pattern)
		}
	}
	flag.VisitAll(func(f *flag.Flag) { fmt.Printf("flag.CommandLine: -%s\n", f.Name) })
}
