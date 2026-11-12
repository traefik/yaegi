package main

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type MyClaims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

func main() {
	key := []byte("secret")
	exp := jwt.NewNumericDate(time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC))
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, MyClaims{"admin", jwt.RegisteredClaims{Subject: "bob", ExpiresAt: exp}})
	s, err := t.SignedString(key)
	fmt.Println(s, err)
	var c MyClaims
	p, err := jwt.ParseWithClaims(s, &c, func(*jwt.Token) (any, error) { return key, nil })
	fmt.Println(err, p.Valid, c.Role, c.Subject)
	m, err := jwt.Parse(s, func(*jwt.Token) (any, error) { return key, nil })
	fmt.Println(err, m.Claims.(jwt.MapClaims)["role"])
}
