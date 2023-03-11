package interp

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// importSrc calls gta on the source code for the package identified by
// importPath. rPath is the relative path to the directory containing the source
// code for the package. It can also be "main" as a special value.
func (interp *Interpreter) importSrc(rPath, importPath string, skipTest bool) (string, error) {
	var dir string
	var err error

	if interp.srcPkg[importPath] != nil {
		name, ok := interp.pkgNames[importPath]
		if !ok {
			return "", fmt.Errorf("inconsistent knowledge about %s", importPath)
		}
		return name, nil
	}

	// For relative import paths in the form "./xxx" or "../xxx", the initial
	// base path is the directory of the interpreter input file, or "." if no file
	// was provided.
	// In all other cases, absolute import paths are resolved from the GOPATH
	// and the nested "vendor" directories.
	if isPathRelative(importPath) {
		if rPath == mainID {
			rPath = "."
		}
		dir = filepath.Join(filepath.Dir(interp.name), rPath, importPath)
	} else if dir, rPath, err = interp.pkgDir(interp.context.GOPATH, rPath, importPath); err != nil {
		// Try again, assuming a root dir at the source location.
		if rPath, err = interp.rootFromSourceLocation(); err != nil {
			return "", err
		}
		if dir, rPath, err = interp.pkgDir(interp.context.GOPATH, rPath, importPath); err != nil {
			return "", err
		}
	}

	if interp.rdir[importPath] {
		return "", fmt.Errorf("import cycle not allowed\n\timports %s", importPath)
	}
	interp.rdir[importPath] = true

	files, err := fs.ReadDir(interp.opt.filesystem, dir)
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
		if skipFile(&interp.context, name, skipTest) {
			continue
		}

		name = filepath.Join(dir, name)
		var buf []byte
		if buf, err = fs.ReadFile(interp.opt.filesystem, name); err != nil {
			return "", err
		}

		n, err := interp.parse(string(buf), name, false)
		if err != nil {
			return "", err
		}
		if n == nil {
			continue
		}

		var pname string
		if pname, root, err = interp.ast(n); err != nil {
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
		} else if pkgName != pname && skipTest {
			return "", fmt.Errorf("found packages %s and %s in %s", pkgName, pname, dir)
		}
		rootNodes = append(rootNodes, root)

		subRPath := effectivePkg(rPath, importPath)
		var list []*node
		list, err = interp.gta(root, subRPath, importPath, pkgName)
		if err != nil {
			return "", err
		}
		revisit[subRPath] = append(revisit[subRPath], list...)
	}

	// Revisit incomplete nodes where GTA could not complete.
	for _, nodes := range revisit {
		if err = interp.gtaRetry(nodes, importPath, pkgName); err != nil {
			return "", err
		}
	}

	// Generate control flow graphs.
	for _, root := range rootNodes {
		var nodes []*node
		if nodes, err = interp.cfg(root, nil, importPath, pkgName); err != nil {
			return "", err
		}
		initNodes = append(initNodes, nodes...)
	}

	// Register source package in the interpreter. The package contains only
	// the global symbols in the package scope.
	interp.mutex.Lock()
	gs := interp.scopes[importPath]
	if gs == nil {
		interp.mutex.Unlock()
		// A nil scope means that no even an empty package is created from source.
		return "", fmt.Errorf("no Go files in %s", dir)
	}
	interp.srcPkg[importPath] = gs.sym
	interp.pkgNames[importPath] = pkgName

	interp.frame.mutex.Lock()
	interp.resizeFrame()
	interp.frame.mutex.Unlock()
	interp.mutex.Unlock()

	// Once all package sources have been parsed, execute entry points then init functions.
	for _, n := range rootNodes {
		if err = genRun(n); err != nil {
			return "", err
		}
		interp.run(n, nil)
	}

	// Wire and execute global vars in global scope gs.
	n, err := genGlobalVars(rootNodes, gs)
	if err != nil {
		return "", err
	}
	interp.run(n, nil)

	// Add main to list of functions to run, after all inits.
	if m := gs.sym[mainID]; pkgName == mainID && m != nil && skipTest {
		initNodes = append(initNodes, m.node)
	}

	for _, n := range initNodes {
		interp.run(n, interp.frame)
	}

	return pkgName, nil
}

// rootFromSourceLocation returns the path to the directory containing the input
// Go file given to the interpreter, relative to $GOPATH/src.
// It is meant to be called in the case when the initial input is a main package.
func (interp *Interpreter) rootFromSourceLocation() (string, error) {
	sourceFile := interp.name
	if sourceFile == DefaultSourceName {
		return "", nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	pkgDir := filepath.Join(wd, filepath.Dir(sourceFile))
	root := strings.TrimPrefix(pkgDir, filepath.Join(interp.context.GOPATH, "src")+"/")
	if root == wd {
		return "", fmt.Errorf("package location %s not in GOPATH", pkgDir)
	}
	return root, nil
}

// pkgDir returns the absolute path in filesystem for a package given its import path
// and the root of the subtree dependencies.
func (interp *Interpreter) pkgDir(goPath string, root, importPath string) (string, string, error) {
	rPath := filepath.Join(root, "vendor")
	dir := filepath.Join(goPath, "src", rPath, importPath)

	if _, err := fs.Stat(interp.opt.filesystem, dir); err == nil {
		return dir, rPath, nil // found!
	}

	dir = filepath.Join(goPath, "src", effectivePkg(root, importPath))

	if _, err := fs.Stat(interp.opt.filesystem, dir); err == nil {
		return dir, root, nil // found!
	}

	if len(root) == 0 {
		if interp.context.GOPATH == "" {
			return "", "", fmt.Errorf("unable to find source related to: %q. Either the GOPATH environment variable, or the Interpreter.Options.GoPath needs to be set", importPath)
		}
		return "", "", fmt.Errorf("unable to find source related to: %q", importPath)
	}

	rootPath := filepath.Join(goPath, "src", root)
	prevRoot, err := previousRoot(interp.opt.filesystem, rootPath, root)
	if err != nil {
		return "", "", err
	}

	return interp.pkgDir(goPath, prevRoot, importPath)
}

const vendor = "vendor"

// Find the previous source root (vendor > vendor > ... > GOPATH).
func previousRoot(filesystem fs.FS, rootPath, root string) (string, error) {
	rootPath = filepath.Clean(rootPath)
	parent, final := filepath.Split(rootPath)
	parent = filepath.Clean(parent)

	// TODO(mpl): maybe it works for the special case main, but can't be bothered for now.
	if root != mainID && final != vendor {
		root = strings.TrimSuffix(root, string(filepath.Separator))
		prefix := strings.TrimSuffix(strings.TrimSuffix(rootPath, root), string(filepath.Separator))

		// look for the closest vendor in one of our direct ancestors, as it takes priority.
		var vendored string
		for {
			fi, err := fs.Stat(filesystem, filepath.Join(parent, vendor))
			if err == nil && fi.IsDir() {
				vendored = strings.TrimPrefix(strings.TrimPrefix(parent, prefix), string(filepath.Separator))
				break
			}
			if !os.IsNotExist(err) {
				return "", err
			}
			// stop when we reach GOPATH/src
			if parent == prefix {
				break
			}

			// stop when we reach GOPATH/src/blah
			parent = filepath.Dir(parent)
			if parent == prefix {
				break
			}

			// just an additional failsafe, stop if we reach the filesystem root, or dot (if
			// we are dealing with relative paths).
			// TODO(mpl): It should probably be a critical error actually,
			// as we shouldn't have gone that high up in the tree.
			// TODO(dennwc): This partially fails on Windows, since it cannot recognize drive letters as "root".
			if parent == string(filepath.Separator) || parent == "." || parent == "" {
				break
			}
		}

		if vendored != "" {
			return vendored, nil
		}
	}

	// TODO(mpl): the algorithm below might be redundant with the one above,
	// but keeping it for now. Investigate/simplify/remove later.
	splitRoot := strings.Split(root, string(filepath.Separator))
	var index int
	for i := len(splitRoot) - 1; i >= 0; i-- {
		if splitRoot[i] == "vendor" {
			index = i
			break
		}
	}

	if index == 0 {
		return "", nil
	}

	return filepath.Join(splitRoot[:index]...), nil
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
// It is intended for use on import paths, where "/" is always the directory separator.
func isPathRelative(s string) bool {
	return strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../")
}
