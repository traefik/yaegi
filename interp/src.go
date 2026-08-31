package interp

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"strings"
)

type sourcePackageInitPhase uint8

const (
	sourcePackagePrepared sourcePackageInitPhase = iota
	sourcePackageFailed
	sourcePackageCommitted
)

// sourcePackageInit keeps the immutable compiler output separate from the
// runtime initialization attempt. GTA and CFG state cannot be rolled back
// after a canceled import, but the prepared nodes can safely initialize a new
// detached root on retry.
type sourcePackageInit struct {
	pkgName    string
	rootNodes  []*node
	globals    *node
	initNodes  []*node
	phase      sourcePackageInitPhase
	generation uint64
	failedRoot *frame
}

type sourcePackageBuild struct {
	cancel <-chan struct{}
}

func canceledOwner(cancel <-chan struct{}) bool {
	if cancel == nil {
		return false
	}
	select {
	case <-cancel:
		return true
	default:
		return false
	}
}

// importSrc calls gta on the source code for the package identified by
// importPath. rPath is the relative path to the directory containing the source
// code for the package. It can also be "main" as a special value.
func (interp *Interpreter) importSrc(rPath, importPath string, skipTest bool) (string, error) {
	return interp.importSrcWithCancel(rPath, importPath, skipTest, nil)
}

func (interp *Interpreter) importSrcWithCancel(rPath, importPath string, skipTest bool, cancel <-chan struct{}) (string, error) {
	interp.compileMu.Lock()
	previousCancel := interp.compileCancel
	interp.compileCancel = cancel
	defer func() {
		interp.compileCancel = previousCancel
		interp.compileMu.Unlock()
	}()
	return interp.importSrcLocked(rPath, importPath, skipTest)
}

// importSrcLocked compiles and initializes a source package. The caller holds
// compileMu; this method releases it only while executing prepared code.
func (interp *Interpreter) importSrcLocked(rPath, importPath string, skipTest bool) (string, error) {
	var dir string
	var err error

	switch importPath {
	case "C":
		return "", fmt.Errorf("cgo is not supported: %q cannot be imported from interpreted code", importPath)
	case "plugin":
		return "", fmt.Errorf("%q cannot be imported from interpreted code: the plugin package is inaccessible to the interpreter", importPath)
	}

	if prepared := interp.srcPkgInit[importPath]; prepared != nil {
		if prepared.phase == sourcePackageCommitted {
			return prepared.pkgName, nil
		}
		return interp.runSourcePackageInitLocked(prepared)
	}
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
		dir = path.Join(path.Dir(interp.name), rPath, importPath)
	} else if dir, rPath, err = interp.pkgDir(filepath.ToSlash(interp.context.GOPATH), rPath, importPath); err != nil {
		// Try again, assuming a root dir at the source location.
		if rPath, err = interp.rootFromSourceLocation(); err != nil {
			return "", err
		}
		if dir, rPath, err = interp.pkgDir(filepath.ToSlash(interp.context.GOPATH), rPath, importPath); err != nil {
			return "", err
		}
	}

	if build := interp.srcPkgBuild[importPath]; build != nil {
		if !canceledOwner(build.cancel) {
			return "", fmt.Errorf("import cycle not allowed\n\timports %s", importPath)
		}
		// A canceled nested initializer can leave its importing package below
		// the prepared-state boundary. Discard only that package's compiler
		// scope and AST/generic products; successfully prepared dependencies
		// retain their own state and can be retried independently.
		interp.rollbackSourcePackageBuildLocked(importPath, build)
	} else if interp.rdir[importPath] {
		return "", fmt.Errorf("import cycle not allowed\n\timports %s", importPath)
	}
	build := &sourcePackageBuild{cancel: interp.compileCancel}
	interp.srcPkgBuild[importPath] = build
	interp.rdir[importPath] = true
	preparedBuild := false
	defer func() {
		if !preparedBuild {
			interp.rollbackSourcePackageBuildLocked(importPath, build)
		}
	}()

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

		name = path.Join(dir, name)
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

	gs := interp.scopes[importPath]
	if gs == nil {
		// A nil scope means that no even an empty package is created from source.
		return "", fmt.Errorf("no Go files in %s", dir)
	}

	// Prepare every runtime node while compiler state is serialized.
	for _, n := range rootNodes {
		if err = genRun(n); err != nil {
			return "", err
		}
	}
	globals, err := genGlobalVars(rootNodes, gs)
	if err != nil {
		return "", err
	}

	// Add main to list of functions to run, after all inits.
	if m := gs.sym[mainID]; pkgName == mainID && m != nil && skipTest {
		initNodes = append(initNodes, m.node)
	}

	prepared := &sourcePackageInit{
		pkgName:   pkgName,
		rootNodes: append([]*node(nil), rootNodes...),
		globals:   globals,
		initNodes: append([]*node(nil), initNodes...),
		phase:     sourcePackagePrepared,
	}

	// Register the prepared source package. Imported symbols must remain
	// available after a canceled or panicking initializer because compiler
	// scopes and indexes are append-only; srcPkgInit controls whether the
	// package may be treated as successfully initialized.
	interp.mutex.Lock()
	interp.srcPkg[importPath] = gs.sym
	interp.pkgNames[importPath] = pkgName
	interp.srcPkgInit[importPath] = prepared
	interp.mutex.Unlock()
	interp.refreshGlobalVarIndexesLocked()
	delete(interp.srcPkgBuild, importPath)
	preparedBuild = true

	return interp.runSourcePackageInitLocked(prepared)
}

func (interp *Interpreter) rollbackSourcePackageBuildLocked(importPath string, build *sourcePackageBuild) {
	if build == nil || interp.srcPkgBuild[importPath] != build || interp.srcPkgInit[importPath] != nil {
		return
	}
	delete(interp.srcPkgBuild, importPath)
	delete(interp.rdir, importPath)
	interp.mutex.Lock()
	delete(interp.scopes, importPath)
	delete(interp.srcPkg, importPath)
	delete(interp.pkgNames, importPath)
	interp.mutex.Unlock()
	interp.refreshGlobalVarIndexesLocked()
	for key, generic := range interp.generic {
		if generic != nil && generic.scope != nil && generic.scope.pkgID == importPath {
			delete(interp.generic, key)
		}
	}
	roots := interp.roots[:0]
	for _, root := range interp.roots {
		if root != nil && root.scope != nil && root.scope.pkgID == importPath {
			continue
		}
		roots = append(roots, root)
	}
	interp.roots = roots
}

// runSourcePackageInitLocked executes prepared package initialization while
// temporarily releasing compileMu. Failed attempts are never cached as
// success; their next attempt runs on a detached root so partial globals from
// cancellation or panic cannot become the package's committed state.
func (interp *Interpreter) runSourcePackageInitLocked(prepared *sourcePackageInit) (string, error) {
	compileOwner := interp.compileCancel
	// Source-package initialization runs interpreted steps on this
	// goroutine; mark it like executeWithPublication so a synchronous host
	// callback of the initializer can re-enter the interpreter. The ambient
	// compile owner is the token's liveness owner: a retry whose owner
	// already fired still runs on the gate-holding goroutine, so the
	// gate-holder rule keeps its reentrancy.
	acquireExecutionToken(interp, compileOwner)
	defer releaseExecutionToken(interp)
	if prepared.phase == sourcePackageFailed && prepared.failedRoot != nil {
		// The detach reads live cells of the failed root, whose canceled
		// worker may still be unwinding deferred calls. Detach under the
		// same exclusive fence prepareExecutionFrame uses, so the walk
		// cannot observe a half-written cell.
		interp.funcSweepMu.Lock()
		interp.mutex.Lock()
		if interp.frame == prepared.failedRoot {
			interp.frame = interp.frame.cloneDetached(compileOwner)
		}
		interp.mutex.Unlock()
		interp.funcSweepMu.Unlock()
	}
	prepared.generation++
	attempt := prepared.generation
	f, err := interp.prepareExecutionFrame(compileOwner)
	if err != nil {
		return "", err
	}

	// User code must not hold the preparation lock: a canceled source-package
	// initializer may remain blocked in native code while a later Eval proceeds.
	interp.compileMu.Unlock()
	var runPanic any
	func() {
		defer func() { runPanic = recover() }()
		active := func() bool { return !f.canceled() }
		for _, root := range prepared.rootNodes {
			interp.runOnFrame(root, f)
			if !active() {
				return
			}
		}
		interp.runOnFrame(prepared.globals, f)
		if !active() {
			return
		}
		for _, initNode := range prepared.initNodes {
			interp.run(initNode, f)
			if !active() {
				return
			}
		}
	}()
	interp.compileMu.Lock()
	interp.compileCancel = compileOwner
	if runPanic != nil {
		if prepared.phase != sourcePackageCommitted && prepared.generation == attempt {
			prepared.phase = sourcePackageFailed
			prepared.failedRoot = f.root
		}
		panic(runPanic)
	}
	if f.canceled() {
		if prepared.phase != sourcePackageCommitted && prepared.generation == attempt {
			prepared.phase = sourcePackageFailed
			prepared.failedRoot = f.root
		}
		return "", context.Canceled
	}
	if prepared.generation == attempt {
		prepared.phase = sourcePackageCommitted
		prepared.failedRoot = nil
	}
	return prepared.pkgName, nil
}

// rootFromSourceLocation returns the path to the directory containing the input
// Go file given to the interpreter, relative to $GOPATH/src.
// It is meant to be called in the case when the initial input is a main package.
func (interp *Interpreter) rootFromSourceLocation() (string, error) {
	sourceFile := interp.name
	if sourceFile == DefaultSourceName {
		return "", nil
	}

	_, isRealFS := interp.opt.filesystem.(*realFS)
	if isRealFS {
		// In the "real" FS, GOPATH will be an absolute path, so we need to convert
		// the source file to an absolute path to compare them.
		absPath, err := filepath.Abs(filepath.FromSlash(sourceFile))
		if err != nil {
			return "", err
		}
		sourceFile = filepath.ToSlash(absPath)
	}
	pkgDir := path.Dir(sourceFile)
	goPath := path.Join(filepath.ToSlash(interp.context.GOPATH), "src") + "/"
	if !strings.HasPrefix(pkgDir, goPath) {
		return "", fmt.Errorf("package location %s not in GOPATH", pkgDir)
	}
	return strings.TrimPrefix(pkgDir, goPath), nil
}

// pkgDir returns the absolute path in filesystem for a package given its import path
// and the root of the subtree dependencies.
func (interp *Interpreter) pkgDir(goPath string, root, importPath string) (string, string, error) {
	rPath := path.Join(root, "vendor")
	dir := path.Join(goPath, "src", rPath, importPath)

	if _, err := fs.Stat(interp.opt.filesystem, dir); err == nil {
		return dir, rPath, nil // found!
	}

	dir = path.Join(goPath, "src", effectivePkg(root, importPath))

	if _, err := fs.Stat(interp.opt.filesystem, dir); err == nil {
		return dir, root, nil // found!
	}

	if root == "" {
		if interp.context.GOPATH == "" {
			return "", "", fmt.Errorf("unable to find source related to: %q. The Interpreter.Options.GoPath needs to be set (the interpreter library does not read the GOPATH environment variable, but the yaegi command does and passes it as GoPath)", importPath)
		}
		return "", "", fmt.Errorf("unable to find source related to: %q", importPath)
	}

	rootPath := path.Join(goPath, "src", root)
	prevRoot, err := previousRoot(interp.opt.filesystem, rootPath, root)
	if err != nil {
		return "", "", err
	}

	return interp.pkgDir(goPath, prevRoot, importPath)
}

const vendor = "vendor"

// Find the previous source root (vendor > vendor > ... > GOPATH).
func previousRoot(filesystem fs.FS, rootPath, root string) (string, error) {
	rootPath = path.Clean(rootPath)
	parent, final := path.Split(rootPath)
	parent = path.Clean(parent)

	// TODO(mpl): maybe it works for the special case main, but can't be bothered for now.
	if root != mainID && final != vendor {
		root = strings.TrimSuffix(root, "/")
		prefix := strings.TrimSuffix(strings.TrimSuffix(rootPath, root), "/")

		// look for the closest vendor in one of our direct ancestors, as it takes priority.
		var vendored string
		for {
			fi, err := fs.Stat(filesystem, path.Join(parent, vendor))
			if err == nil && fi.IsDir() {
				vendored = strings.TrimPrefix(strings.TrimPrefix(parent, prefix), "/")
				break
			}
			if !errors.Is(err, fs.ErrNotExist) {
				return "", err
			}
			// stop when we reach GOPATH/src
			if parent == prefix {
				break
			}

			// stop when we reach GOPATH/src/blah
			parent = path.Dir(parent)
			if parent == prefix {
				break
			}

			// just an additional failsafe, stop if we reach the filesystem root, or dot (if
			// we are dealing with relative paths).
			// TODO(mpl): It should probably be a critical error actually,
			// as we shouldn't have gone that high up in the tree.
			// TODO(dennwc): This partially fails on Windows, since it cannot recognize drive letters as "root".
			if parent == "/" || parent == "." || parent == "" {
				break
			}
		}

		if vendored != "" {
			return vendored, nil
		}
	}

	// TODO(mpl): the algorithm below might be redundant with the one above,
	// but keeping it for now. Investigate/simplify/remove later.
	splitRoot := strings.Split(root, "/")
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

	return path.Join(splitRoot[:index]...), nil
}

func effectivePkg(root, p string) string {
	splitRoot := strings.Split(root, "/")
	splitPath := strings.Split(p, "/")

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
		frag = path.Join(frag, result[i])
	}

	return path.Join(root, frag)
}

// isPathRelative returns true if path starts with "./" or "../".
// It is intended for use on import paths, where "/" is always the directory separator.
func isPathRelative(s string) bool {
	return strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../")
}
