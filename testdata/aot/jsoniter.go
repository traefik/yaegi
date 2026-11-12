// AOT test program: github.com/json-iterator/go on interpreted types
// (reflect2 reaches the runtime with linkname: the VM emulates it,
// vm/reflect2.go).
//
//aot:lib github.com/json-iterator/go
package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	jsoniter "github.com/json-iterator/go"
)

type Level int

func (l Level) MarshalJSON() ([]byte, error) { return []byte(strconv.Quote("L" + strconv.Itoa(int(l)))), nil }

type Item struct {
	Name  string            `json:"name"`
	Level Level             `json:"level"`
	Tags  []string          `json:"tags,omitempty"`
	Meta  map[string]string `json:"meta"`
}

var json = jsoniter.ConfigCompatibleWithStandardLibrary

func main() {
	if benchMain(benchOp) {
		return
	}
	b, err := json.Marshal(Item{Name: "a", Level: 2, Meta: map[string]string{"k": "v"}})
	fmt.Println(string(b), err)
	var it Item
	err = json.Unmarshal([]byte(`{"name":"b","tags":["x"],"meta":{"z":"1"}}`), &it)
	fmt.Println(err, it)
	s, _ := json.MarshalToString(map[string]any{"n": 1, "l": []any{true, nil}})
	fmt.Println(s)
	fmt.Println(jsoniter.Get([]byte(`{"a":{"b":[1,2]}}`), "a", "b", 1).ToInt())
}

// benchOp: marshal and unmarshal an item.
func benchOp() {
	b, err := json.Marshal(Item{Name: "a", Level: 2, Meta: map[string]string{"k": "v"}})
	if err != nil {
		panic(err)
	}
	var it Item
	if err := json.Unmarshal(b[:0], &it); err == nil {
		panic("expected error")
	}
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
