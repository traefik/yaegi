package main

import (
	"encoding/json"
	"fmt"
	"time"
)

type Date struct{ time.Time }

type Claims struct {
	Sub string `json:"sub"`
	Exp *Date  `json:"exp,omitempty"`
}

func main() {
	_, err := json.Marshal(Claims{Sub: "a", Exp: &Date{time.Unix(42, 0)}})
	fmt.Println(err)

	_, err = json.Marshal([]Date{{time.Unix(42, 0)}})
	fmt.Println(err)

	d := Date{time.Unix(42, 0).UTC()}
	fmt.Println(d.Unix(), d.Year())
}

// Output:
// <nil>
// <nil>
// 42 1970
