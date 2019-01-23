package interp

import (
	"io/ioutil"
	"os"
	"path/filepath"
)

func (i *Interpreter) importSrcFile(path string) error {
	dir, err := pkgDir(i.GoPath, path)
	if err != nil {
		return err
	}

	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}

	var initNodes []*Node
	var rootNodes []*Node

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
		var buf []byte
		if buf, err = ioutil.ReadFile(name); err != nil {
			return err
		}

		if _, root, err = i.ast(string(buf), name); err != nil {
			return err
		}
		rootNodes = append(rootNodes, root)

		if i.AstDot {
			root.AstDot(DotX(), name)
		}

		if err = i.Gta(root); err != nil {
			return err
		}
	}

	// Generate control flow graphs
	for _, root := range rootNodes {
		var nodes []*Node
		if nodes, err = i.Cfg(root); err != nil {
			return err
		}
		initNodes = append(initNodes, nodes...)
	}

	if i.NoRun {
		return nil
	}

	// Once all package sources have been parsed, execute entry points then init functions
	for _, n := range rootNodes {
		if err = genRun(n); err != nil {
			return err
		}
		i.fsize++
		i.resizeFrame()
		i.run(n, nil)
	}

	for _, n := range initNodes {
		i.run(n, i.Frame)
	}
	return nil
}

// pkgDir returns the absolute path in filesystem for a package given its name
func pkgDir(goPath string, path string) (string, error) {
	dir := filepath.Join(goPath, "src", path, "vendor")
	if _, err := os.Stat(dir); err == nil {
		return dir, nil // found!
	}

	dir = filepath.Join(goPath, "src", path)
	_, err := os.Stat(dir)

	return dir, err
}
