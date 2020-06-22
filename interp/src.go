package interp

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

func (interp *Interpreter) importSrc(rPath, path string) (string, error) {
	var dir string
	var err error

	if interp.srcPkg[path] != nil {
		return interp.pkgNames[path], nil
	}

	// For relative import paths in the form "./xxx" or "../xxx", the initial
	// base path is the directory of the interpreter input file, or "." if no file
	// was provided.
	// In all other cases, absolute import paths are resolved from the GOPATH
	// and the nested "vendor" directories.
	if isPathRelative(path) {
		if rPath == "main" {
			rPath = "."
		}
		dir = filepath.Join(filepath.Dir(interp.Name), rPath, path)
	} else if dir, rPath, err = pkgDir(interp.context.GOPATH, rPath, path); err != nil {
		return "", err
	}

	if interp.rdir[path] {
		return "", fmt.Errorf("import cycle not allowed\n\timports %s", path)
	}
	interp.rdir[path] = true

	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return "", err
	}

	var initNodes []*node
	var rootNodes []*node
	revisit := make(map[string][]*node)

	var root *node
	var pkgName string

	// Parse source files.
	for _, file := range files {
		name := file.Name()
		if skipFile(&interp.context, name) {
			continue
		}

		name = filepath.Join(dir, name)
		var buf []byte
		if buf, err = ioutil.ReadFile(name); err != nil {
			return "", err
		}

		var pname string
		if pname, root, err = interp.ast(string(buf), name); err != nil {
			return "", err
		}
		if root == nil {
			continue
		}

		if interp.astDot {
			dotCmd := interp.dotCmd
			if dotCmd == "" {
				dotCmd = defaultDotCmd(name, "yaegi-ast-")
			}
			root.astDot(dotWriter(dotCmd), name)
		}
		if pkgName == "" {
			pkgName = pname
		} else if pkgName != pname {
			return "", fmt.Errorf("found packages %s and %s in %s", pkgName, pname, dir)
		}
		rootNodes = append(rootNodes, root)

		subRPath := effectivePkg(rPath, path)
		var list []*node
		list, err = interp.gta(root, subRPath, path)
		if err != nil {
			return "", err
		}
		revisit[subRPath] = append(revisit[subRPath], list...)
	}

	// Revisit incomplete nodes where GTA could not complete.
	for pkg, nodes := range revisit {
		if err = interp.gtaRetry(nodes, pkg, path); err != nil {
			return "", err
		}
	}

	// Generate control flow graphs
	for _, root := range rootNodes {
		var nodes []*node
		if nodes, err = interp.cfg(root, path); err != nil {
			return "", err
		}
		initNodes = append(initNodes, nodes...)
	}

	// Register source package in the interpreter. The package contains only
	// the global symbols in the package scope.
	interp.mutex.Lock()
	interp.srcPkg[path] = interp.scopes[path].sym
	interp.pkgNames[path] = pkgName

	interp.frame.mutex.Lock()
	interp.resizeFrame()
	interp.frame.mutex.Unlock()
	interp.mutex.Unlock()

	// Once all package sources have been parsed, execute entry points then init functions
	for _, n := range rootNodes {
		if err = genRun(n); err != nil {
			return "", err
		}
		interp.run(n, nil)
	}

	// Add main to list of functions to run, after all inits
	if m := interp.main(); m != nil {
		initNodes = append(initNodes, m)
	}

	for _, n := range initNodes {
		interp.run(n, interp.frame)
	}

	return pkgName, nil
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

// Find the previous source root (vendor > vendor > ... > GOPATH).
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

		index := len(splitRoot) - 1 - rootIndex
		if index > 0 && part == splitRoot[index] && i != 0 {
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

// isPathRelative returns true if path starts with "./" or "../".
func isPathRelative(s string) bool {
	p := "." + string(filepath.Separator)
	return strings.HasPrefix(s, p) || strings.HasPrefix(s, "."+p)
}
