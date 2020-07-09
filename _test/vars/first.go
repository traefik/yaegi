package vars

var (
	A = concat("hello", B)
	C = D
)

func concat(a ...string) string {
	var s string
	for _, ss := range a {
		s += ss
	}
	return s
}
