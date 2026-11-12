// Package mapclaims decodes token claims like github.com/golang-jwt/jwt/v5
// (parser.go): the claims of the caller are a non nil MapClaims in an
// interface, decoded with json.Unmarshal or json.Decoder.Decode through a
// pointer to a copy of the map. The decoded claims must be seen through the
// interface.
package mapclaims

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

type Claims interface {
	GetAudience() (string, error)
}

type MapClaims map[string]any

func (m MapClaims) GetAudience() (string, error) {
	aud, ok := m["aud"].(string)
	if !ok {
		return "", fmt.Errorf("aud: claim is not present")
	}
	return aud, nil
}

type Token struct{ Claims Claims }

func parse(data []byte, claims Claims, useNumber bool) (*Token, error) {
	token := &Token{Claims: claims}
	var err error
	if c, ok := token.Claims.(MapClaims); ok {
		if useNumber {
			dec := json.NewDecoder(bytes.NewReader(data))
			dec.UseNumber()
			err = dec.Decode(&c)
		} else {
			err = json.Unmarshal(data, &c)
		}
	} else {
		err = json.Unmarshal(data, &claims)
	}
	return token, err
}

type Config struct{}

func CreateConfig() *Config { return &Config{} }

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var out []string
		for _, useNumber := range []bool{false, true} {
			tok, err := parse([]byte(`{"aud":"traefik","exp":1}`), MapClaims{}, useNumber)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			aud, err := tok.Claims.GetAudience()
			if err != nil {
				http.Error(w, err.Error(), http.StatusForbidden)
				return
			}
			var keys []string
			for k := range tok.Claims.(MapClaims) {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			out = append(out, aud+" "+strings.Join(keys, ","))
		}
		fmt.Fprint(w, strings.Join(out, ";"))
	}), nil
}
