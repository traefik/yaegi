// reflect.Value.Pointer of a native map is a uintptr, and a uintptr is a
// valid map key: the code that walks values with reflect keeps the visited
// maps by their address (mergo, deep copies). The VM keeps the native value
// with the address, and used the map itself as the key: "hash of unhashable
// type".
package main

import (
	"fmt"
	"net/http"
	"reflect"
)

func main() {
	h := http.Header{"A": {"1"}}
	visited := map[uintptr]bool{}
	visited[reflect.ValueOf(h).Pointer()] = true
	fmt.Println(visited[reflect.ValueOf(h).Pointer()], len(visited))
	visited[reflect.ValueOf(http.Header{}).Pointer()] = true
	fmt.Println(len(visited))
}
