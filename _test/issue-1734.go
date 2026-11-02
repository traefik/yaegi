package main

import (
	"encoding/json"
	"fmt"
)

type T struct{ n int }

func (t T) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`{"num": %d}`, t.n)), nil
}

func (t *T) UnmarshalJSON(b []byte) error {
	t.n = len(b)
	return nil
}

func main() {
	var m json.Marshaler = T{3}
	b, err := json.Marshal(m)
	fmt.Println(string(b), err)

	t := &T{}
	var u json.Unmarshaler = t
	err = u.UnmarshalJSON([]byte(`{"num": 3}`))
	fmt.Println(t.n, err)
}

// Output:
// {"num":3} <nil>
// 10 <nil>
