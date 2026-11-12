// encoding/gob and encoding/json decode a named []byte type of the program
// (the copies of gob and json make it with reflect.MakeSlice and read into
// its Bytes).
package main

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"fmt"
)

type NB []byte

type B byte

type BS []B

type Msg struct {
	B  NB
	S  BS
	NS []NB
}

func main() {
	var b bytes.Buffer
	in := Msg{B: NB("bytes"), S: BS{1, 2, 3}, NS: []NB{NB("x"), nil, NB("yz")}}
	fmt.Println(gob.NewEncoder(&b).Encode(in))
	var out Msg
	fmt.Println(gob.NewDecoder(&b).Decode(&out))
	fmt.Printf("%q %v %q\n", out.B, out.S, out.NS)

	b.Reset()
	fmt.Println(gob.NewEncoder(&b).Encode(NB("top")))
	var nb NB
	fmt.Println(gob.NewDecoder(&b).Decode(&nb))
	fmt.Printf("%q\n", nb)

	b.Reset()
	gob.NewEncoder(&b).Encode(in)
	pre := Msg{B: NB{9, 9, 9, 9, 9, 9, 9, 9}}
	fmt.Println(gob.NewDecoder(&b).Decode(&pre))
	fmt.Printf("%q %v\n", pre.B, pre.S)

	j, err := json.Marshal(in)
	fmt.Println(string(j), err)
	var jo Msg
	fmt.Println(json.Unmarshal(j, &jo))
	fmt.Printf("%q %v %q\n", jo.B, jo.S, jo.NS)
}
