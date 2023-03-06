package interp

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"
)

// importSrc calls global tag analysis on the source code for the package identified by
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

	// resolve relative and absolute import paths.
	if dir, err = interp.getPackageDir(importPath); err != nil {
		return "", err
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

// getPackageDir uses the provided Go module environment variables to find the absolute path of an import path.
func (interp *Interpreter) getPackageDir(importPath string) (string, error) {
	// ensure that an absolute GOPATH is used.
	absGoPath, err := filepath.Abs(interp.context.GOPATH)
	if err != nil {
		return "", fmt.Errorf("an error occurred determining the absolute path of a GOPATH %q: %w", interp.context.GOPATH, err)
	}

	// load the package.
	config := packages.Config{
		Env: []string{
			"GOPATH=" + absGoPath,
			"GOCACHE=" + interp.opt.env["GOCACHE"],
			"GOROOT=" + interp.context.GOROOT,
			"GOPRIVATE=" + interp.opt.env["GOPRIVATE"],
			"GOMODCACHE=" + interp.opt.env["GOMODCACHE"],
			"GO111MODULE=" + interp.opt.env["GO111MODULE"],
		},
	}

	pkgs, err := packages.Load(&config, importPath)
	if err != nil {
		return "", fmt.Errorf("an error occurred retrieving a package: %v\n%v\nIf Access is denied, run in administrator", importPath, err)
	}

	// confirm the import path is found.
	for _, pkg := range pkgs {
		for _, goFile := range pkg.GoFiles {
			if strings.Contains(filepath.Dir(goFile), pkg.Name) {
				return filepath.Dir(goFile), nil
			}
		}
	}

	return "", fmt.Errorf("an import source could not be found for %q", importPath)
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
