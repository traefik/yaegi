package vin

import (
	"fmt"
	
	"guthib.com/containous/cheese"
)

func Hello() string {
	return fmt.Sprintf("vin %s", cheese.Hello())
}