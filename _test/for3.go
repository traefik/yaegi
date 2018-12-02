package main

import "fmt"

func main() {
	//m := map[string][]string{
	//	"hello": {"foo", "bar"},
	//	"world": {"truc", "machin"},
	//}
	m := map[string][]string{
		"hello": []string{"foo", "bar"},
		"world": []string{"truc", "machin"},
	}
	//fmt.Println(m)
	for key, values := range m {
		//fmt.Println(key, values)
		for _, value := range values {
			fmt.Println(key, value)
		}
	}
}
