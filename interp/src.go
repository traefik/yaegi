package interp

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

func (i *Interpreter) importSrcFile(rPath, path, alias string) error {
	var dir string
	var err error

	// For relative import paths in the form "./xxx" or "../xxx", the initial
	// base path is the directory of the interprer input file, or "." if no file
	// was provided.
	// In all other cases, absolute import paths are resolved from the GOPATH
	// and the nested "vendor" directories.
	if isPathRelative(path) {
		if rPath == "main" {
			rPath = "."
		}
		dir = filepath.Join(filepath.Dir(i.Name), rPath, path)
	} else if dir, rPath, err = pkgDir(i.GoPath, rPath, path); err != nil {
		return err
	}

	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}

	var initNodes []*Node
	var rootNodes []*Node

	var root *Node
	var pkgName string

	// Parse source files
	for _, file := range files {
		name := file.Name()
		if skipFile(name) {
			continue
		}

		name = filepath.Join(dir, name)
		var buf []byte
		if buf, err = ioutil.ReadFile(name); err != nil {
			return err
		}

		var pname string
		if pname, root, err = i.ast(string(buf), name); err != nil {
			return err
		}
		if root == nil {
			continue
		}
		if pkgName == "" {
			pkgName = pname
		} else if pkgName != pname {
			return fmt.Errorf("found packages %s and %s in %s", pkgName, pname, dir)
		}
		rootNodes = append(rootNodes, root)

		if i.AstDot {
			root.AstDot(DotX(), name)
		}

		subRPath := effectivePkg(rPath, path)
		if err = i.Gta(root, subRPath); err != nil {
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

	// Rename imported pkgName to alias if they are different
	if pkgName != alias {
		i.scope[alias] = i.scope[pkgName]
		delete(i.scope, pkgName)
	}

	if i.NoRun {
		return nil
	}

	i.resizeFrame()

	// Once all package sources have been parsed, execute entry points then init functions
	for _, n := range rootNodes {
		if err = genRun(n); err != nil {
			return err
		}
		i.run(n, nil)
	}

	for _, n := range initNodes {
		i.run(n, i.Frame)
	}

	return err
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

	if _, err := os.Stat(dir); err == nil {
		return dir, root, nil // found!
	}

	if len(root) == 0 {
		return "", "", fmt.Errorf("unable to find source related to: %q", path)
	}

	return pkgDir(goPath, previousRoot(root), path)
}

// Find the previous source root. (vendor > vendor > ... > GOPATH)
func previousRoot(root string) string {
	splitRoot := strings.Split(root, string(filepath.Separator))

	var index int
	for i := len(splitRoot) - 1; i >= 0; i-- {
		if splitRoot[i] == "vendor" {
			index = i
			break
		}
	}

	if index == 0 {
		return ""
	}

	return filepath.Join(splitRoot[:index]...)
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

// isPathRelative returns true if path starts with "./" or "../"
func isPathRelative(s string) bool {
	p := "." + string(filepath.Separator)
	return strings.HasPrefix(s, p) || strings.HasPrefix(s, "."+p)
}
