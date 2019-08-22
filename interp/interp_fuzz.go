// +build gofuzz

package interp

func Fuzz(input []byte) int {
	interpreter := New(Options{})
	_, err := interpreter.Eval(string(input))
	if err != nil {
		return 1
	}
	return 0
}
