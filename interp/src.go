package interp

import (
	"go/build"
	"io/ioutil"
	"os"
	"path/filepath"
)

func (i *Interpreter) importSrcFile(path string) error {
	dir, err := pkgDir(path)
	if err != nil {
		return err
	}

	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}

	initNodes := []*Node{}
	rootNodes := []*Node{}

	var root *Node

	// Parse source files
	for _, file := range files {
		name := file.Name()
		if len(name) <= 3 || name[len(name)-3:] != ".go" {
			continue
		}
		if len(name) > 8 && name[len(name)-8:] == "_test.go" {
			continue
		}

		name = filepath.Join(dir, name)
		buf, err := ioutil.ReadFile(name)
		if err != nil {
			return err
		}

		_, root, err = i.ast(string(buf), name)
		if err != nil {
			return err
		}
		rootNodes = append(rootNodes, root)
		if i.AstDot {
			root.AstDot(DotX(), name)
		}
		err = i.Gta(root)
		if err != nil {
			return err
		}
	}

	// Generate control flow graphs
	for _, root := range rootNodes {
		nodes, err := i.Cfg(root)
		if err != nil {
			return err
		}
		initNodes = append(initNodes, nodes...)
	}

	if i.NoRun {
		return nil
	}

	// Once all package sources have been parsed, execute entry points then init functions
	for _, n := range rootNodes {
		genRun(n)
		i.fsize++
		i.resizeFrame()
		i.run(n, nil)
	}

	for _, n := range initNodes {
		i.run(n, i.Frame)
	}
	return nil
}

// pkgDir returns the abolute path in filesystem for a package given its name
func pkgDir(path string) (string, error) {
	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		return dir, err
	}

	dir = filepath.Join(dir, "vendor", path)
	_, err = os.Stat(dir)
	if err == nil {
		return dir, err
	}

	dir = filepath.Join(build.Default.GOPATH, "src", path)
	_, err = os.Stat(dir)

	return dir, err
}
