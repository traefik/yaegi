// A program with its own generated protobuf messages (vm/testdata/pbplug):
// Marshal, Unmarshal, Equal, Clone, Merge, Reset, Any, protojson,
// prototext and protoreflect, then round trips on 8 goroutines
// (docs/UNSAFE.md).
package main

import (
	"fmt"
	"sync"

	"github.com/traefik/yaegi/v2/vm/testdata/pbplug"
)

func main() {
	fmt.Print(pbplug.Report())
	var wg sync.WaitGroup
	sizes := make([]int, 8)
	for g := range sizes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				sizes[g] += pbplug.Roundtrip()
			}
		}()
	}
	wg.Wait()
	fmt.Println("roundtrips", sizes)
}
