package main

func main() {
	s := make([]map[string]string, 0)
	m := make(map[string]string)
	m["m1"] = "m1"
	m["m2"] = "m2"
	s = append(s, m)
	tmpStr := "start"
	println(tmpStr)
	for _, v := range s {
		tmpStr, ok := v["m1"]
		println(tmpStr, ok)
	}
	println(tmpStr)
}

// Output:
// start
// m1 true
// start
