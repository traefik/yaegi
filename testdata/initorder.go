// The packages are initialized in the order of the Go spec (since Go
// 1.21): sorted by import path, each one as soon as its imports are
// initialized, not in the order of the import declarations (GOROOT test
// fixedbugs/issue31636.go). Package a imports strings, whose path sorts
// after the others: b, c and d come first.
package main

import (
	"fmt"

	"github.com/traefik/yaegi/v2/vm/testdata/initorder/d"

	_ "github.com/traefik/yaegi/v2/vm/testdata/initorder/c"

	"github.com/traefik/yaegi/v2/vm/testdata/initorder/b"

	"github.com/traefik/yaegi/v2/vm/testdata/initorder/a"
)

func init() { println("init main") }

func main() {
	fmt.Println(a.A, b.B, d.D)
}
