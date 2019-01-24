package fromage

import (
	"fmt"

	"github.com/foo/pkg/fromage/cheese"
)

func Hello() string {
	return fmt.Sprintf("Fromage %s", cheese.Hello())
}
