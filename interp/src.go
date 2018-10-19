package interp

import (
	"go/build"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"reflect"
)

func (interp *Interpreter) importSrcFile(path string) {
	dir := pkgDir(path)

	files, err := ioutil.ReadDir(dir)
	if err != nil {
		log.Fatal(err)
	}

	initNodes := []*Node{}
	rootNodes := []*Node{}

	var root *Node
	//var pkgName string

	// Parse source files
	for _, file := range files {
		name := file.Name()
		if len(name) <= 3 || name[len(name)-3:] != ".go" {
			continue
		}
		if len(name) > 8 && name[len(name)-8:] == "_test.go" {
			continue
		}

		//log.Println("src", name)
		buf, err := ioutil.ReadFile(filepath.Join(dir, name))
		if err != nil {
			log.Fatal(err)
		}

		_, root = interp.Ast(string(buf))
		rootNodes = append(rootNodes, root)
		if interp.AstDot {
			root.AstDot(DotX())
		}
		interp.Gta(root)
	}

	// Generate control flow graphs
	for _, root := range rootNodes {
		initNodes = append(initNodes, interp.Cfg(root)...)
	}

	if interp.NoRun {
		return
	}

	// Once all package sources have been parsed, execute entry points then init functions
	for _, n := range rootNodes {
		genRun(n)
		interp.fsize++
		interp.resizeFrame()
		// Allocate frame values
		for i, t := range root.types {
			if t != nil {
				interp.Frame.data[i] = reflect.New(t).Elem()
			}
		}
		runCfg(n.start, interp.Frame)
	}

	for _, n := range initNodes {
		run0(n, interp.Frame)
	}
}

// pkgDir returns the abolute path in filesystem for a package given its name
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
