// Concurrent encoding/json on json.RawMessage fields. In -aot mode the AOT
// copy of encoding/json reads and writes these fields through shadows of
// the VM cells (vm/twin_value.go, shadowRef). A shadow belongs to the
// goroutine that made it:
//   - a native to VM transition of one goroutine must not give back the
//     shadows that the AOT code of another goroutine is still writing
//     (Keys came back empty);
//   - a read only json.Marshal of a shared value must not write its cells
//     (keys came back null, and the shared value lost them for good).
//
//aot:lib encoding/json
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"
)

type Config struct {
	Name string          `json:"name"`
	Keys json.RawMessage `json:"keys"`
}

var shared = Config{Name: "shared", Keys: json.RawMessage(`[4,5]`)}

func main() {
	if benchMain(benchOp) {
		return
	}
	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				var doc struct {
					Keys json.RawMessage `json:"keys"`
				}
				if err := json.Unmarshal([]byte(`{"keys":[1,2,3]}`), &doc); err != nil || string(doc.Keys) != "[1,2,3]" {
					panic(fmt.Sprint(err, string(doc.Keys)))
				}
			}
		}()
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				b, err := json.Marshal(&shared)
				if err != nil || string(b) != `{"name":"shared","keys":[4,5]}` {
					panic(fmt.Sprint(err, string(b)))
				}
			}
		}()
	}
	wg.Wait()
	b, _ := json.Marshal(shared)
	fmt.Println(string(b))
	fmt.Println("ok")
}

// benchOp: decode into a json.RawMessage field, encode the shared config.
func benchOp() {
	var doc struct {
		Keys json.RawMessage `json:"keys"`
	}
	json.Unmarshal([]byte(`{"keys":[1,2,3]}`), &doc)
	json.Marshal(&shared)
}

func benchMain(op func()) bool {
	n, _ := strconv.Atoi(os.Getenv("AOT_BENCH"))
	if n <= 0 {
		return false
	}
	op()
	t0 := time.Now()
	for i := 0; i < n; i++ {
		op()
	}
	fmt.Printf("BENCH %d\n", time.Since(t0).Nanoseconds()/int64(n))
	return true
}
