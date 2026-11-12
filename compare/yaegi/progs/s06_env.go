package main

import (
	"fmt"
	"os"
)

func main() {
	os.Setenv("CAPAB_PLUGIN_SET", "1")
	fmt.Println(os.Getenv("CAPAB_PLUGIN_SET"), os.Getenv("HOME") != "")
}
