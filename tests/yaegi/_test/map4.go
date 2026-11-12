package main

func main() {
	dict := map[string]string{"bidule": "machin", "truc": "bidule"}
	dict["hello"] = "bonjour"
	println(dict["bidule"])
	println(dict["hello"])
}

// Output:
// machin
// bonjour
