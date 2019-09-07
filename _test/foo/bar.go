package foo

import "./boo"

var Bar = "BARR"
var Boo = boo.Boo

func init() { println("init foo") }
