package fromage

import (
	"fmt"

	"guthib.com/traefik/cheese"
	"guthib.com/traefik/fromage/couteau"
)

func Hello() string {
	return fmt.Sprintf("Fromage %s %s", couteau.Hello(), cheese.Hello())
}
