package foo

import bar "github.com/traefik/yaegi/_test/b2/foo"

var Desc = "in b1/foo"

var Desc2 = Desc + bar.Desc
