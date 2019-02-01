package cheese

import (
	"fmt"

	"guthib.com/containous/fromage"
)

func Hello() string {
	return fmt.Sprintf("cheese %s", fromage.Hello())
}