// github.com/golang-jwt/jwt/v5: HS256 tokens with an exp claim, custom
// claims with RegisteredClaims embedded, MapClaims, NumericDate with a
// fraction, expired and not yet valid tokens, a wrong key.
package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	User  string   `json:"user"`
	Roles []string `json:"roles"`
	jwt.RegisteredClaims
}

var key = []byte("test-secret-key-only-for-tests")

func sign(c jwt.Claims) string {
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(key)
	if err != nil {
		panic(err)
	}
	return s
}

func keyFunc(t *jwt.Token) (any, error) {
	if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
		return nil, fmt.Errorf("unexpected method %v", t.Header["alg"])
	}
	return key, nil
}

func main() {
	fixed := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	c := Claims{User: "ann", Roles: []string{"admin"}, RegisteredClaims: jwt.RegisteredClaims{
		Issuer: "me", Subject: "s1", Audience: jwt.ClaimStrings{"api"}, ExpiresAt: jwt.NewNumericDate(fixed), IssuedAt: jwt.NewNumericDate(fixed.AddDate(-10, 0, 0)),
	}}
	tok := sign(c)
	fmt.Println(tok)

	var got Claims
	t, err := jwt.ParseWithClaims(tok, &got, keyFunc, jwt.WithAudience("api"), jwt.WithIssuer("me"))
	fmt.Println(err, t.Valid, got.User, got.Roles, got.ExpiresAt.Time.UTC(), got.Audience)
	exp, err := t.Claims.GetExpirationTime()
	fmt.Println(exp.Unix(), err)

	// Expired.
	old := sign(jwt.MapClaims{"user": "bob", "exp": 1000})
	_, err = jwt.Parse(old, keyFunc)
	fmt.Println(err, errors.Is(err, jwt.ErrTokenExpired))

	// exp with a fraction, as a float in MapClaims.
	frac := sign(jwt.MapClaims{"exp": float64(fixed.Unix()) + 0.5, "nbf": 1.25})
	t, err = jwt.Parse(frac, keyFunc)
	fmt.Println(err, t.Valid)
	mc := t.Claims.(jwt.MapClaims)
	e, _ := mc.GetExpirationTime()
	n, _ := mc.GetNotBefore()
	fmt.Println(e.UnixMilli(), n.UnixMilli(), mc["exp"])

	// Not yet valid.
	nyv := sign(jwt.MapClaims{"nbf": fixed.Unix()})
	_, err = jwt.Parse(nyv, keyFunc)
	fmt.Println(errors.Is(err, jwt.ErrTokenNotValidYet))

	// Wrong key and wrong algorithm.
	_, err = jwt.Parse(tok, func(*jwt.Token) (any, error) { return []byte("other"), nil })
	fmt.Println(err, errors.Is(err, jwt.ErrSignatureInvalid))
	_, err = jwt.Parse(tok, keyFunc, jwt.WithValidMethods([]string{"RS256"}))
	fmt.Println(err)

	// exp as a string: invalid type.
	bad := sign(jwt.MapClaims{"exp": "tomorrow"})
	_, err = jwt.Parse(bad, keyFunc)
	fmt.Println(err, errors.Is(err, jwt.ErrInvalidType))

	// Missing exp, required.
	none := sign(jwt.MapClaims{"user": "x"})
	_, err = jwt.Parse(none, keyFunc, jwt.WithExpirationRequired())
	fmt.Println(err)
}
