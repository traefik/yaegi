package cheese

import (
	"fmt"

	"guthib.com/containous/cheese/vin"
)

func Hello() string {
	return fmt.Sprintf("Cheese %s", 	vin.Hello())
}
