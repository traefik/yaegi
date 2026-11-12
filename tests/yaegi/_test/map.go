package main

type Dict map[string]string

func main() {
	dict := make(Dict)
	dict["truc"] = "machin"
	println(dict["truc"])
}

// Output:
// machin
