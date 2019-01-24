package pkg

import (
	"fmt"

	"github.com/foo/pkg/fromage"
)

func Here() string {
	return "root"
}

func NewSample() func() string {
	return func() string {
		return fmt.Sprintf("%s %s", Here(), fromage.Hello())
	}
}
