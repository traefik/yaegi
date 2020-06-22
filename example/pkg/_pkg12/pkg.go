package pkg

import (
	"fmt"
)

func NewSample() func() string {
	return func() string {
		return fmt.Sprintf("gomod!")
	}
}