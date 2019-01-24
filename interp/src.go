package interp

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

func (i *Interpreter) importSrcFile(rPath, path string) error {
	dir, rPath, err := pkgDir(i.GoPath, rPath, path)
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

		subRPath := effectivePkg(rPath, path)
		if i.Gta(root, subRPath) != nil {
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
		if genRun(n) != nil {
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

// pkgDir returns the absolute path in filesystem for a package given its name and
// the root of the subtree dependencies.
func pkgDir(goPath string, root, path string) (string, string, error) {
	rPath := filepath.Join(root, "vendor")
	dir := filepath.Join(goPath, "src", rPath, path)

	if _, err := os.Stat(dir); err == nil {
		return dir, rPath, nil // found!
	}

	dir = filepath.Join(goPath, "src", effectivePkg(root, path))

	_, err := os.Stat(dir)
	return dir, root, err
}

func effectivePkg(root, path string) string {
	splitRoot := strings.Split(root, string(filepath.Separator))
	splitPath := strings.Split(path, string(filepath.Separator))

	var result []string

	rootIndex := 0
	prevRootIndex := 0
	for i := 0; i < len(splitPath); i++ {
		part := splitPath[len(splitPath)-1-i]

		if part == splitRoot[len(splitRoot)-1-rootIndex] {
			prevRootIndex = rootIndex
			rootIndex++
		} else if prevRootIndex == rootIndex {
			result = append(result, part)
		}
	}

	var frag string
	for i := len(result) - 1; i >= 0; i-- {
		frag = filepath.Join(frag, result[i])
	}

	return filepath.Join(root, frag)
}
