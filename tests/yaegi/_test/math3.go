package main

import (
	"crypto/md5"
	"fmt"
)

func md5Crypt(password, salt, magic []byte) []byte {
	d := md5.New()
	d.Write(password)
	d.Write(magic)
	d.Write(salt)

	d2 := md5.New()
	d2.Write(password)
	d2.Write(salt)

	for i, mixin := 0, d2.Sum(nil); i < len(password); i++ {
		d.Write([]byte{mixin[i%16]})
	}

	return d.Sum(nil)
}

func main() {
	b := md5Crypt([]byte("1"), []byte("2"), []byte("3"))

	fmt.Println(b)
}

// Output:
// [187 141 73 89 101 229 33 106 226 63 117 234 117 149 230 21]
