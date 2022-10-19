package named3

import (
	"fmt"
	"net/http"
)

type T struct {
	A string
}

func (t *T) Print() {
	println(t.A)
}

type A http.Header

func (a A) ForeachKey() error {
	for k, vals := range a {
		for _, v := range vals {
			fmt.Println(k, v)
		}

	}

	return nil
}

func (a A) Set(k string, v []string) {
	a[k] = v
}
