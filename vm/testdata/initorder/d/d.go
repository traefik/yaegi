// Package d imports package c (testdata/initorder.go).
package d

import "github.com/traefik/yaegi/v2/vm/testdata/initorder/c"

var D = c.C + "d"

func init() { println("init", D) }
