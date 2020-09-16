package cheese

import (
	"fmt"

	"guthib.com/traefik/fromage"
)

func Hello() string {
	return fmt.Sprintf("cheese %s", fromage.Hello())
}
