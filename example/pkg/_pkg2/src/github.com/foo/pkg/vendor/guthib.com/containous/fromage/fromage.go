package fromage

import (
	"fmt"

	"guthib.com/containous/cheese"
)

func Hello() string {
	return fmt.Sprintf("Fromage %s", cheese.Hello())
}