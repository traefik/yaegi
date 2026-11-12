package main

func main() {
	dict := map[string]string{"bidule": "machin"}
	for k, v := range dict {
		println(k, v)
	}
}

// Output:
// bidule machin
