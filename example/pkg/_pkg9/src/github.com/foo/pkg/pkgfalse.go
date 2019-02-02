package pkgfalse

import (
	"fmt"

	"github.com/foo/pkg/fromage"
)

func HereNot() string {
	return "root"
}

func NewSampleNot() func() string {
	return func() string {
		return fmt.Sprintf("%s %s", HereNot(), fromage.Here())
	}
}
