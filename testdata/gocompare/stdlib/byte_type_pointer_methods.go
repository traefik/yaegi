// A pointer to an element of a []U, U a byte type of the program, has the
// methods of *U for json and text/template (the []U is kept as Go bytes,
// and the pointer is a *byte).
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"text/template"
)

type U byte

func (u *U) UnmarshalJSON(b []byte) error {
	var n byte
	fmt.Sscan(string(b), &n)
	*u = U(n + 100)
	return nil
}

func (u *U) Inc() U { *u++; return *u }

func (u U) String() string { return fmt.Sprintf("U%d", byte(u)) }

func main() {
	one := make([]U, 2)
	fmt.Println(json.Unmarshal([]byte(`7`), &one[1]), one)
	us := make([]U, 2)
	fmt.Println(json.Unmarshal([]byte(`[1,2,3]`), &us), us)
	var m map[string]U
	fmt.Println(json.Unmarshal([]byte(`{"a":4}`), &m), m)
	t := template.Must(template.New("").Parse("{{range .L}}{{.Inc}} {{end}}\n"))
	d := struct{ L []U }{[]U{1, 2}}
	fmt.Println(t.Execute(os.Stdout, &d), d.L)
	b, err := json.Marshal(us)
	fmt.Println(string(b), err)
}
