package vin

import (
	"fmt"

	"guthib.com/traefik/cheese"
)

func Hello() string {
	return fmt.Sprintf("vin %s", cheese.Hello())
}
