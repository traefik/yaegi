package interp

import (
	"go/build"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
)

func (interp *Interpreter) importSrcFile(path string) {
	dir := pkgDir(path)
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		log.Fatal(err)
	}
	for _, file := range files {
		name := file.Name()
		if len(name) <= 3 || name[len(name)-3:] != ".go" {
			continue
		}
		if len(name) > 8 && name[len(name)-8:] == "_test.go" {
			continue
		}
		buf, err := ioutil.ReadFile(filepath.Join(dir, name))
		if err != nil {
			log.Fatal(err)
		}
		interp.Eval(string(buf))
	}
}

func pkgDir(path string) string {
	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		log.Fatal(err)
	}
	dir = filepath.Join(dir, "vendor", path)
	if _, err := os.Stat(dir); err == nil {
		return dir
	}
	dir = filepath.Join(build.Default.GOPATH, "src", path)
	if _, err := os.Stat(dir); err != nil {
		log.Fatal(err)
	}
	return dir
}
