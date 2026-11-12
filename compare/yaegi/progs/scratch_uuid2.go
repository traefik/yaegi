package main

import (
	"crypto/sha1"
	"fmt"

	"github.com/google/uuid"
)

func main() {
	fmt.Println(uuid.NameSpaceDNS.String())
	u := uuid.NewHash(sha1.New(), uuid.NameSpaceDNS, []byte("traefik.io"), 5)
	fmt.Println(u.String())
	fmt.Println(uuid.Version(5))
	var v uuid.Version = 5
	fmt.Println(v.String())
}
