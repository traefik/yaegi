package interp

import (
	"context"
	"io/ioutil"
	"reflect"
	"runtime"
	"runtime/debug"
)

// A Program is Go code that has been parsed and compiled.
type Program struct {
	pkgName string
	root    *node
	init    []*node
}

// Compile parses and compiles a Go code represented as a string.
func (interp *Interpreter) Compile(src string) (*Program, error) {
	return interp.compile(src, "", true)
}

// CompilePath parses and compiles a Go code located at the given path.
func (interp *Interpreter) CompilePath(path string) (*Program, error) {
	if !isFile(interp.filesystem, path) {
		_, err := interp.importSrc(mainID, path, NoTest)
		return nil, err
	}

	b, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return interp.compile(string(b), path, false)
}

func (interp *Interpreter) compile(src, name string, inc bool) (*Program, error) {
	if name != "" {
		interp.name = name
	}
	if interp.name == "" {
		interp.name = DefaultSourceName
	}

	// Parse source to AST.
	pkgName, root, err := interp.ast(src, interp.name, inc)
	if err != nil || root == nil {
		return nil, err
	}

	if interp.astDot {
		dotCmd := interp.dotCmd
		if dotCmd == "" {
			dotCmd = defaultDotCmd(interp.name, "yaegi-ast-")
		}
		root.astDot(dotWriter(dotCmd), interp.name)
		if interp.noRun {
			return nil, err
		}
	}

	// Perform global types analysis.
	if err = interp.gtaRetry([]*node{root}, pkgName, pkgName); err != nil {
		return nil, err
	}

	// Annotate AST with CFG informations.
	initNodes, err := interp.cfg(root, pkgName, pkgName)
	if err != nil {
		if interp.cfgDot {
			dotCmd := interp.dotCmd
			if dotCmd == "" {
				dotCmd = defaultDotCmd(interp.name, "yaegi-cfg-")
			}
			root.cfgDot(dotWriter(dotCmd))
		}
		return nil, err
	}

	if root.kind != fileStmt {
		// REPL may skip package statement.
		setExec(root.start)
	}
	interp.mutex.Lock()
	gs := interp.scopes[pkgName]
	if interp.universe.sym[pkgName] == nil {
		// Make the package visible under a path identical to its name.
		interp.srcPkg[pkgName] = gs.sym
		interp.universe.sym[pkgName] = &symbol{kind: pkgSym, typ: &itype{cat: srcPkgT, path: pkgName}}
		interp.pkgNames[pkgName] = pkgName
	}
	interp.mutex.Unlock()

	// Add main to list of functions to run, after all inits.
	if m := gs.sym[mainID]; pkgName == mainID && m != nil {
		initNodes = append(initNodes, m.node)
	}

	if interp.cfgDot {
		dotCmd := interp.dotCmd
		if dotCmd == "" {
			dotCmd = defaultDotCmd(interp.name, "yaegi-cfg-")
		}
		root.cfgDot(dotWriter(dotCmd))
	}

	return &Program{pkgName, root, initNodes}, nil
}

// Execute executes compiled Go code.
func (interp *Interpreter) Execute(p *Program) (res reflect.Value, err error) {
	defer func() {
		r := recover()
		if r != nil {
			var pc [64]uintptr // 64 frames should be enough.
			n := runtime.Callers(1, pc[:])
			err = Panic{Value: r, Callers: pc[:n], Stack: debug.Stack()}
		}
	}()

	// Generate node exec closures.
	if err = genRun(p.root); err != nil {
		return res, err
	}

	// Init interpreter execution memory frame.
	interp.frame.setrunid(interp.runid())
	interp.frame.mutex.Lock()
	interp.resizeFrame()
	interp.frame.mutex.Unlock()

	// Execute node closures.
	interp.run(p.root, nil)

	// Wire and execute global vars.
	n, err := genGlobalVars([]*node{p.root}, interp.scopes[p.pkgName])
	if err != nil {
		return res, err
	}
	interp.run(n, nil)

	for _, n := range p.init {
		interp.run(n, interp.frame)
	}
	v := genValue(p.root)
	res = v(interp.frame)

	// If result is an interpreter node, wrap it in a runtime callable function.
	if res.IsValid() {
		if n, ok := res.Interface().(*node); ok {
			res = genFunctionWrapper(n)(interp.frame)
		}
	}

	return res, err
}

// ExecuteWithContext executes compiled Go code.
func (interp *Interpreter) ExecuteWithContext(ctx context.Context, p *Program) (res reflect.Value, err error) {
	interp.mutex.Lock()
	interp.done = make(chan struct{})
	interp.cancelChan = !interp.opt.fastChan
	interp.mutex.Unlock()

	done := make(chan struct{})
	go func() {
		defer close(done)
		res, err = interp.Execute(p)
	}()

	select {
	case <-ctx.Done():
		interp.stop()
		return reflect.Value{}, ctx.Err()
	case <-done:
	}
	return res, err
}
