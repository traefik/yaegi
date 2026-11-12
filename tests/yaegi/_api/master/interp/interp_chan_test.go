package interp_test

import (
	"reflect"
	"testing"

	"github.com/traefik/yaegi/interp"
)

// IntChan is a channel type alias for testing.
type IntChan chan int

// NewIntChan creates a new IntChan.
func NewIntChan() IntChan {
	return make(IntChan, 1)
}

func TestSendToBinaryChannelTypeAlias(t *testing.T) {
	i := interp.New(interp.Options{})

	err := i.Use(interp.Exports{
		"mypkg/mypkg": {
			"IntChan":    reflect.ValueOf((*IntChan)(nil)),
			"NewIntChan": reflect.ValueOf(NewIntChan),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = i.Eval(`
package main

import "mypkg"

func main() {
	ch := mypkg.NewIntChan()
	ch <- 42
	val := <-ch
	if val != 42 {
		panic("unexpected value")
	}
}
`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSendToSourceDefinedChannel(t *testing.T) {
	i := interp.New(interp.Options{})

	// Test with a channel defined purely in interpreted code
	_, err := i.Eval(`
package main

func main() {
	ch := make(chan int, 1)
	ch <- 42
	val := <-ch
	if val != 42 {
		panic("unexpected value")
	}
}
`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSendToSourceDefinedChannelTypeAlias(t *testing.T) {
	i := interp.New(interp.Options{})

	// Test with a channel type alias defined in interpreted code
	_, err := i.Eval(`
package main

type MyChan chan int

func main() {
	ch := make(MyChan, 1)
	ch <- 42
	val := <-ch
	if val != 42 {
		panic("unexpected value")
	}
}
`)
	if err != nil {
		t.Fatal(err)
	}
}
