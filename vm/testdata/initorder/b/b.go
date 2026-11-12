// Package b has no imports (testdata/initorder.go).
package b

var B = trace("b")

func trace(s string) string {
	println("var", s)
	return s
}

func init() { println("init", B) }
