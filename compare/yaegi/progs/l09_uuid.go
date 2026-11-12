package main

import (
	"fmt"

	"github.com/google/uuid"
)

func main() {
	u := uuid.NewSHA1(uuid.NameSpaceDNS, []byte("traefik.io"))
	fmt.Println(u, u.Version())
	p, err := uuid.Parse(u.String())
	fmt.Println(p == u, err)
	fmt.Println(len(uuid.NewString()))
}
