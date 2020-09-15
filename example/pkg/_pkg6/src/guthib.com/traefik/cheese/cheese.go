package cheese

import (
	"fmt"

	"guthib.com/traefik/cheese/vin"
)

func Hello() string {
	return fmt.Sprintf("Cheese %s", vin.Hello())
}
