package main

// encoding/gob in default mode runs as its AOT copy (stdAOTModes in
// vm/aot.go): native gob ignored GobEncode of the program's types and cut
// the recursive type User, so it wrote other bytes than Go, with no error.
// (From the reflect battery of the modes challenge, case gob.)

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"strings"
)

type User struct {
	Username      string
	Authenticated bool
	Tags          []string
	Meta          map[string]int
	Next          *User
}

type Shape interface{ Name() string }

type Square struct{ Side int }

func (s Square) Name() string { return fmt.Sprintf("square %d", s.Side) }

type Holder struct {
	S Shape
	A any
}

type Secret struct{ v string }

func (s Secret) GobEncode() ([]byte, error) { return []byte(strings.ToUpper(s.v)), nil }
func (s *Secret) GobDecode(b []byte) error  { s.v = strings.ToLower(string(b)); return nil }

type Box struct {
	S Secret
	N int
}

func main() {
	gob.Register(User{})
	gob.Register(Square{})

	var buf bytes.Buffer
	err := gob.NewEncoder(&buf).Encode(map[string]any{"user": User{"user42", true, nil, nil, nil}})
	fmt.Printf("%q %v\n", buf.Bytes(), err)

	buf.Reset()
	u := User{Username: "a", Tags: []string{"x"}, Meta: map[string]int{"k": 1}, Next: &User{Username: "b"}}
	err = gob.NewEncoder(&buf).Encode(u)
	fmt.Println(buf.Len(), err)
	var u2 User
	err = gob.NewDecoder(&buf).Decode(&u2)
	fmt.Println(err, u2.Username, u2.Tags, u2.Meta, u2.Next != nil && u2.Next.Username == "b")

	buf.Reset()
	err = gob.NewEncoder(&buf).Encode(Holder{S: Square{3}, A: User{Username: "in any"}})
	fmt.Printf("%q %v\n", buf.Bytes(), err)
	var h Holder
	err = gob.NewDecoder(&buf).Decode(&h)
	hu, ok := h.A.(User)
	fmt.Println(err, h.S != nil && h.S.Name() == "square 3", ok, hu.Username)

	buf.Reset()
	err = gob.NewEncoder(&buf).Encode(Box{Secret{"hidden"}, 7})
	fmt.Printf("%q %v\n", buf.Bytes(), err)
	var bx Box
	err = gob.NewDecoder(&buf).Decode(&bx)
	fmt.Println(err, bx.S.v, bx.N)
}
