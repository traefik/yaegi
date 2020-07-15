package pkg

import (
	"fmt"

	"guthib.com/bar"
)

func Here() string {
	return "hello"
}

func NewSample() func() string {
	return func() string {
		return fmt.Sprintf("%s %s", bar.Bar(), Here())
	}
}
