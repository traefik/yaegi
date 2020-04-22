package main

import (
	"encoding/json"
	"strconv"
)

func main() {
	jb := []byte(`{"num": "2"}`)
	params := map[string]interface{}{"foo": "1"}
	if err := json.Unmarshal(jb, &params); err != nil {
		panic(err)
	}
	sum := 0
	for _, v := range params {
		i, err := strconv.Atoi(v.(string))
		if err != nil {
			panic(err)
		}
		sum += i
	}
	println(sum)
}

// Output:
// 3
