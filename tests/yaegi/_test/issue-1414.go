package main

import (
	"encoding/json"
	"fmt"
)

func main() {
	arr := make([]interface{}, 0)
	arr = append(arr, []interface{}{1, 2, 3})
	arr = append(arr, []interface{}{5, 6, 7})
	bytes, _ := json.Marshal(arr)
	fmt.Println("array: " + string(bytes))
}

// Output:
// array: [[1,2,3],[5,6,7]]
