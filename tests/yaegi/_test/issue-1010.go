package main

import (
	"encoding/json"
	"fmt"
)

type MyJsonMarshaler struct{ n int }

func (m MyJsonMarshaler) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`{"num": %d}`, m.n)), nil
}

func main() {
	ch := make(chan json.Marshaler, 1)
	ch <- MyJsonMarshaler{2}
	m, err := json.Marshal(<-ch)
	fmt.Println(string(m), err)
}

// Output:
// {"num":2} <nil>
