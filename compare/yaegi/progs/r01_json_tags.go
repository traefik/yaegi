package main

import (
	"encoding/json"
	"fmt"
)

type Backend struct {
	URL    string `json:"url"`
	Weight *int   `json:"weight,omitempty"`
}

type Meta struct {
	Owner string `json:"owner"`
}

type Cfg struct {
	Meta
	Name     string            `json:"name"`
	Backends []Backend         `json:"backends"`
	Headers  map[string]string `json:"headers,omitempty"`
	Next     *Cfg              `json:"next,omitempty"`
	internal int
}

func main() {
	w := 3
	c := Cfg{Meta: Meta{"me"}, Name: "a", Backends: []Backend{{"http://x", &w}, {URL: "http://y"}}, Next: &Cfg{Name: "b"}}
	b, err := json.Marshal(c)
	fmt.Println(string(b), err)
	var d Cfg
	err = json.Unmarshal([]byte(`{"owner":"o","name":"n","backends":[{"url":"u","weight":7}],"headers":{"k":"v"},"next":{"name":"m"}}`), &d)
	fmt.Println(err, d.Owner, d.Name, d.Backends[0].URL, *d.Backends[0].Weight, d.Headers, d.Next.Name)
	var anyv map[string]any
	json.Unmarshal(b, &anyv)
	fmt.Println(anyv["name"], anyv["owner"])
}
