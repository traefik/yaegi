package fromage

import (
	"fmt"

	"guthib.com/traefik/cheese"
)

func Hello() string {
	return fmt.Sprintf("Fromage %s", cheese.Hello())
}
