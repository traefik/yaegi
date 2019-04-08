package p1

import "math/rand"

var Uint32 = rand.Uint32

func init() { rand.Seed(1) }
