package interp

import (
	"io/ioutil"
	"log"
	"os"
)

func (interp *Interpreter) importSrcFile(path string) {
	basedir := os.Getenv("HOME") + "/go/src/"
	dir := basedir + path
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
		buf, err := ioutil.ReadFile(dir + "/" + name)
		if err != nil {
			log.Fatal(err)
		}
		pkgName, sdef := interp.Eval(string(buf))
		if interp.srcPkg[pkgName] == nil {
			s := make(NodeMap)
			interp.srcPkg[pkgName] = &s
		}
		for name, node := range *sdef {
			(*interp.srcPkg[pkgName])[name] = node
		}
	}
}
