package main

import (
	"fmt"
	"strings"

	jsoniter "github.com/json-iterator/go"
)

type Level int

func (l Level) MarshalJSON() ([]byte, error) { return []byte(`"L` + fmt.Sprint(int(l)) + `"`), nil }

type Upper string

func (u *Upper) UnmarshalJSON(b []byte) error {
	*u = Upper(strings.ToUpper(strings.Trim(string(b), `"`)))
	return nil
}

type Cfg struct {
	Name  string   `json:"name"`
	Tags  []string `json:"tags,omitempty"`
	Level Level    `json:"level"`
	Who   Upper    `json:"who"`
	Skip  int      `json:"-"`
}

func main() {
	j := jsoniter.ConfigCompatibleWithStandardLibrary
	b, err := j.Marshal(Cfg{Name: "a", Tags: []string{"x"}, Level: 3, Who: "w"})
	fmt.Println(string(b), err)
	var c Cfg
	err = j.Unmarshal([]byte(`{"name":"n","level":1,"who":"bob","tags":["p","q"]}`), &c)
	fmt.Printf("%+v %v\n", c, err)
	fmt.Println(j.Get([]byte(`{"a":{"b":[1,2,3]}}`), "a", "b", 1).ToInt())
}
