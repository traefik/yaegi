package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
)

// The address of a big [n]byte in real bytes (bigByteArray in
// byteview.go) taken by reflect (Value.Addr) must be a pointer to the
// array, as &k: a store through it changes k, and a load copies k.
// encoding/json takes the address of an addressable value of a named type
// to find a pointer method (MarshalJSON of *Key: condAddrEncoder).

type Key [64]byte

func (k *Key) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	_, err := hex.Decode(k[:], []byte(s))
	return err
}

func (k *Key) MarshalJSON() ([]byte, error) {
	return json.Marshal(hex.EncodeToString(k[:4]))
}

type Config struct {
	Name string
	Key  Key
}

func main() {
	var k [64]byte
	q := reflect.ValueOf(&k).Elem().Addr().Interface().(*[64]byte)
	q[1] = 7
	fmt.Println("store:", k[1], q == &k)
	c := *q
	k[2] = 9
	fmt.Println("load is a copy:", c[1], c[2], k[2])

	rv := reflect.ValueOf(&k).Elem()
	fmt.Println("pointer:", rv.Addr().Pointer() == reflect.ValueOf(&k).Pointer(),
		rv.UnsafeAddr() == reflect.ValueOf(&k).Pointer())

	var cfg Config
	err := json.Unmarshal([]byte(`{"Name":"a","Key":"0102030405"}`), &cfg)
	fmt.Println("json unmarshal:", err, cfg.Name, cfg.Key[:6])
	var key Key
	err = json.Unmarshal([]byte(`"ff00ff"`), &key)
	fmt.Println("json key:", err, key[:4])
	out, err := json.Marshal(&key)
	fmt.Println("json marshal:", string(out), err)

	p1, p2 := new([128]byte), new([128]byte)
	fmt.Println("deepequal:", reflect.DeepEqual(p1, p2))
	p2[127] = 1
	fmt.Println("deepequal differ:", reflect.DeepEqual(p1, p2))
}
