package main

import (
	"encoding/base64"
	"fmt"
)

func main() {
	var buf [4]byte
	//fmt.Println(buf)
	s := base64.RawStdEncoding.EncodeToString(buf)
	//fmt.Println(base64.RawStdEncoding)
	fmt.Println(s)
}
