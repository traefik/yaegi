package main

import (
	"flag"
	"fmt"
	"go/importer"
	"log"
	"os"
	"unicode"
)

func main() {
	log.SetFlags(log.Lshortfile)
	flag.Usage = func() {
		fmt.Println("Usage:", os.Args[0], "pkgname")
		flag.PrintDefaults()
	}
	flag.Parse()
	args := flag.Args()
	if len(args) != 1 {
		log.Fatal("Invalid number of arguments")
	}

	pkg, err := importer.Default().Import(args[0])
	if err != nil {
		log.Fatal(err)
	}
	sc := pkg.Scope()
	for _, name := range sc.Names() {
		// Skip private symboles
		if r := []rune(name); unicode.IsLower(r[0]) {
			continue
		}
		o := sc.Lookup(name)
		fmt.Println(name, o)
	}
}
