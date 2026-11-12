package main

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// Interpreted values with methods, as seen by native fmt and encoding/json.

type Code int

func (c Code) Error() string { return fmt.Sprintf("code %d", int(c)) }

type Name struct{ First, Last string }

func (n Name) String() string { return n.First + " " + n.Last }

type Raw struct{ n int }

func (r Raw) MarshalJSON() ([]byte, error) { return []byte(fmt.Sprintf(`{"num":%d}`, r.n)), nil }

type H struct{ hits int }

func (h *H) ServeHTTP(http.ResponseWriter, *http.Request) {}

func main() {
	c := Code(7)
	fmt.Printf("%v|%s|%d|%x|%q|%5.2v|\n", c, c, c, c, c, c)
	fmt.Println(c, fmt.Sprint(c), fmt.Errorf("wrap: %w", c))

	n := Name{"Ada", "Lovelace"}
	fmt.Printf("%v|%+v|%-12s|\n", n, n, n)
	b, err := json.Marshal(n)
	fmt.Println(string(b), err)
	b, err = json.Marshal(&n)
	fmt.Println(string(b), err)

	b, err = json.Marshal(Raw{2})
	fmt.Println(string(b), err)
	fmt.Println(Raw{3})

	var x any = Raw{4}
	b, _ = json.Marshal([]any{x, n, c})
	fmt.Println(string(b))

	fmt.Printf("%v %+v\n", &H{1}, &H{2})
}
