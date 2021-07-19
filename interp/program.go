package interp

import (
	"context"
	"go/token"
	"io/ioutil"
	"reflect"
	"runtime"
	"runtime/debug"
)

type Statement interface {
	Position(*Interpreter) token.Position
}

func (n *node) Position(i *Interpreter) token.Position { return i.fset.Position(n.pos) }

type Frame interface {
	Previous() Frame
	Variables() []reflect.Value
}

func (f *frame) Previous() Frame {
	if f.anc == nil {
		return nil
	}
	return f.anc
}

func (f *frame) Variables() []reflect.Value {
	return f.data
}

type Debugger interface {
	Exec(Statement, Frame)
}

type Program struct {
	interp  *Interpreter
	pkgName string
	root    *node
	init    []*node
	dbg     Debugger
}

func (p *Program) SetDebugger(dbg Debugger) {
	p.dbg = dbg
}

func (interp *Interpreter) Compile(src string) (*Program, error) {
	return interp.compile(src, "", true)
}

func (interp *Interpreter) CompilePath(path string) (*Program, error) {
	if !isFile(path) {
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
	if err = interp.gtaRetry([]*node{root}, pkgName); err != nil {
		return nil, err
	}

	// Annotate AST with CFG informations.
	initNodes, err := interp.cfg(root, pkgName)
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

	return &Program{interp, pkgName, root, initNodes, nil}, nil
}

func (interp *Interpreter) Execute(p *Program) (res reflect.Value, err error) {
	if p.interp != interp {
		panic("cannot execute a program compiled by a different interpreter")
	}

	defer func() {
		r := recover()
		if r != nil {
			var pc [64]uintptr // 64 frames should be enough.
			n := runtime.Callers(1, pc[:])
			err = Panic{Value: r, Callers: pc[:n], Stack: debug.Stack()}
		}
	}()

	interp.frame.prog = p

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
