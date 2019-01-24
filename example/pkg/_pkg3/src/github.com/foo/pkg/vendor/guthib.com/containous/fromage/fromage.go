package fromage

import (
	"fmt"

	"guthib.com/containous/cheese"
	"guthib.com/containous/fromage/couteau"
)

func Hello() string {
	return fmt.Sprintf("Fromage %s %s", couteau.Hello(), cheese.Hello())
}