package main

import (
	"encoding/json"
	"fmt"
)

func main() {
	jb := []byte(`{"property": "test"}`)
	params := map[string]interface{}{"foo": 1}
	if err := json.Unmarshal(jb, &params); err != nil {
		panic("marshal failed.")
	}
	fmt.Println(params["foo"], params["property"])
}

// Output:
// 1 test
