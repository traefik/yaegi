package interp

import (
	"context"
	"go/ast"
	"go/token"
	"io/fs"
	"path/filepath"
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

// PackageName returns name used in a package clause.
func (p *Program) PackageName() string {
	return p.pkgName
}

// FileSet is the fileset that must be used for parsing Go that will be passed
// to interp.CompileAST().
func (interp *Interpreter) FileSet() *token.FileSet {
	return interp.fset
}

// Compile parses and compiles a Go code represented as a string.
//
// If the source imports source packages, their initialization is executed
// during compilation. Like Eval, compilation is serialized with other
// evaluations on the same interpreter.
func (interp *Interpreter) Compile(src string) (*Program, error) {
	release := interp.acquireExecution()
	defer release()
	return interp.compileSrc(src, "", true)
}

// CompilePath parses and compiles a Go code located at the given path.
func (interp *Interpreter) CompilePath(path string) (*Program, error) {
	path = filepath.ToSlash(path) // Ensure path is in Unix format. Since we work with fs.FS, we need to use Unix format.
	if !isFile(interp.filesystem, path) {
		// A directory imports the source package, which runs its package
		// initialization as interpreted code: take the execution gate so it
		// cannot overlap an in-flight evaluation. A caller that already
		// holds the gate (for example a host callback of a paused
		// evaluation) is recognized as reentrant. Only a file path yields
		// an executable Program; a directory returns a nil Program.
		release := interp.acquireExecution()
		defer release()
		_, err := interp.importSrc(mainID, path, NoTest)
		return nil, err
	}

	b, err := fs.ReadFile(interp.filesystem, path)
	if err != nil {
		return nil, err
	}
	// Compiling a file can also import source packages and run their
	// initialization, so it is serialized like the directory branch.
	release := interp.acquireExecution()
	defer release()
	return interp.compileSrc(string(b), path, false)
}

func (interp *Interpreter) compileSrc(src, name string, inc bool) (*Program, error) {
	return interp.compileSrcWithCancel(src, name, inc, nil)
}

func (interp *Interpreter) compileSrcWithCancel(src, name string, inc bool, cancel <-chan struct{}) (*Program, error) {
	interp.compileMu.Lock()
	previousCancel := interp.compileCancel
	interp.compileCancel = cancel
	defer func() {
		interp.compileCancel = previousCancel
		interp.compileMu.Unlock()
	}()
	return interp.compileSrcLocked(src, name, inc)
}

// compileSrcLocked compiles source while compileMu is held by the caller.
func (interp *Interpreter) compileSrcLocked(src, name string, inc bool) (*Program, error) {
	if name != "" {
		interp.name = name
	}
	if interp.name == "" {
		interp.name = DefaultSourceName
	}

	// Parse source to AST.
	n, err := interp.parse(src, interp.name, inc)
	if err != nil {
		return nil, err
	}

	return interp.compileAST(n)
}

// CompileAST builds a Program for the given Go code AST. Files and block
// statements can be compiled, as can most expressions. Var declaration nodes
// cannot be compiled.
//
// WARNING: The node must have been parsed using interp.FileSet(). Results are
// unpredictable otherwise.
func (interp *Interpreter) CompileAST(n ast.Node) (*Program, error) {
	// Like Compile: gta may import source packages and run their
	// initialization, which must not overlap an in-flight evaluation.
	release := interp.acquireExecution()
	defer release()
	interp.compileMu.Lock()
	defer interp.compileMu.Unlock()
	// Source-package initialization adopts the ambient compileCancel as its
	// execution owner. A reentrant CompileAST from inside a canceled run must
	// not inherit that closed cancel, or the package-init retry would fail
	// spuriously with context.Canceled.
	compileCancel := interp.compileCancel
	interp.compileCancel = nil
	defer func() { interp.compileCancel = compileCancel }()
	return interp.compileAST(n)
}

func (interp *Interpreter) compileAST(n ast.Node) (*Program, error) {
	// Convert AST.
	pkgName, root, err := interp.ast(n)
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
	initNodes, err := interp.cfg(root, nil, pkgName, pkgName)
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
	interp.refreshGlobalVarIndexesLocked()

	// Add main to the functions to run only when this program declares it.
	// Incremental evaluation retains package symbols, so consulting gs alone
	// would replay an older main after every later, unrelated Eval.
	if f, ok := n.(*ast.File); ok && pkgName == mainID {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || fn.Name.Name != mainID {
				continue
			}
			if m := gs.sym[mainID]; m != nil {
				initNodes = append(initNodes, m.node)
			}
			break
		}
	}

	if interp.cfgDot {
		dotCmd := interp.dotCmd
		if dotCmd == "" {
			dotCmd = defaultDotCmd(interp.name, "yaegi-cfg-")
		}
		root.cfgDot(dotWriter(dotCmd))
	}

	return &Program{pkgName: pkgName, root: root, init: initNodes}, nil
}

// Execute executes compiled Go code.
func (interp *Interpreter) Execute(p *Program) (res reflect.Value, err error) {
	release := interp.acquireExecution()
	defer release()
	return interp.execute(p, nil)
}

type executionPublication struct {
	ready     chan struct{}
	decision  chan bool
	requested bool
	accepted  bool
}

func newExecutionPublication() *executionPublication {
	return &executionPublication{ready: make(chan struct{}), decision: make(chan bool, 1)}
}

// request blocks at the exact execution boundary until the context-facing API
// accepts or rejects publication. It is idempotent so a panic during accepted
// finalization cannot start a second handshake from the recovery path.
func (p *executionPublication) request() bool {
	if p == nil {
		return true
	}
	if p.requested {
		return p.accepted
	}
	p.requested = true
	close(p.ready)
	p.accepted = <-p.decision
	return p.accepted
}

func (interp *Interpreter) execute(p *Program, cancel <-chan struct{}) (res reflect.Value, err error) {
	return interp.executeWithPublication(p, cancel, nil)
}

func (interp *Interpreter) executeWithPublication(p *Program, cancel <-chan struct{}, publication *executionPublication) (res reflect.Value, err error) {
	// Mark this goroutine as running the execution so a synchronous host
	// callback can re-enter the interpreter without waiting on the gate,
	// while Evals from unrelated goroutines (including `go`-statement
	// goroutines) still wait.
	acquireExecutionToken(interp, cancel)
	defer releaseExecutionToken(interp)
	// Compiler state, generated global wiring, and global-frame resizing share
	// scopes and type tables. Keep them in one short serialized setup phase;
	// interpreted execution itself remains unlocked so a later Eval can proceed
	// after canceling a worker blocked in native code.
	interp.compileMu.Lock()
	setupLocked := true
	var executionFrame *frame
	defer func() {
		if setupLocked {
			interp.compileMu.Unlock()
		}
		if executionFrame != nil {
			executionFrame.mutex.Lock()
			executionFrame.callArgs = nil
			executionFrame.mutex.Unlock()
		}
		r := recover()
		if r != nil {
			panicValue, token := splitInterpretedPanic(r)
			publish := publication.request()
			if publish {
				interp.publishOwnedPanicToken(token)
			}
			interp.rollbackOwnedPublishedPanic(r)
			if publish {
				interp.markOwnedValuesHostShared(reflect.ValueOf(panicValue))
				interp.preserveReturnedInterpretedFuncs(reflect.ValueOf(panicValue))
			}
			if executionFrame != nil {
				interp.sweepRootOwnedChannels(executionFrame.root)
				if !publish {
					interp.sweepRootInterpretedFuncs(executionFrame.root, reflect.Value{})
				}
				interp.sweepRootOwnedObjects(executionFrame.root)
			}
			var pc [64]uintptr // 64 frames should be enough.
			n := runtime.Callers(1, pc[:])
			err = Panic{Value: panicValue, Callers: pc[:n], Stack: debug.Stack()}
		}
	}()

	// Generate node exec closures.
	if err = genRun(p.root); err != nil {
		return res, err
	}

	f, err := interp.prepareExecutionFrame(cancel)
	if err != nil {
		return res, err
	}
	executionFrame = f
	interp.funcMu.Lock()
	f.funcGroup = &funcMetaGroup{root: f.root}
	interp.funcMu.Unlock()
	active := func() bool { return !f.canceled() }

	// Generate global variable wiring while compiler state is still protected.
	n, err := genGlobalVars([]*node{p.root}, interp.scopes[p.pkgName])
	if err != nil {
		return res, err
	}
	setupLocked = false
	interp.compileMu.Unlock()

	// Execute node closures.
	interp.runOnFrame(p.root, f)
	if !active() {
		return res, err
	}

	// Execute global variable initialization.
	interp.runOnFrame(n, f)
	if !active() {
		return res, err
	}

	for _, n := range p.init {
		interp.run(n, f)
		if !active() {
			return res, err
		}
	}
	v := genValue(p.root)
	res = v(f)
	if !active() {
		return reflect.Value{}, err
	}
	// A statement or void call has no result: return the zero Value rather
	// than a stale value left in the root result slot by a previous eval.
	if p.root.typ == nil {
		res = reflect.Value{}
	}

	// If result is an interpreter node, wrap it in a runtime callable function.
	if res.IsValid() {
		if n, ok := res.Interface().(*node); ok {
			res = genFunctionWrapper(n)(f)
		}
	}
	if !publication.request() {
		interp.sweepRootOwnedChannels(f)
		interp.sweepRootInterpretedFuncs(f, reflect.Value{})
		interp.sweepRootOwnedObjects(f)
		return reflect.Value{}, context.Canceled
	}
	interp.markOwnedValuesHostShared(res)
	interp.sweepRootOwnedChannels(f)
	interp.sweepRootInterpretedFuncs(f, res)
	interp.sweepRootOwnedObjects(f)

	return res, err
}

// prepareExecutionFrame selects, resizes, and binds the exact root for an
// execution. The caller must hold compileMu through this preparation phase.
func (interp *Interpreter) prepareExecutionFrame(cancel <-chan struct{}) (*frame, error) {
	if cancel != nil {
		select {
		case <-cancel:
			return nil, context.Canceled
		default:
		}
	}
	interp.funcSweepMu.Lock()
	defer interp.funcSweepMu.Unlock()
	interp.mutex.Lock()
	if cancel == nil {
		cancel = interp.done
		if cancel == nil {
			interp.done = make(chan struct{})
			cancel = interp.done
		} else {
			select {
			case <-cancel:
				interp.done = make(chan struct{})
				cancel = interp.done
			default:
			}
		}
	} else {
		interp.done = cancel
	}
	if interp.frame.canceled() {
		interp.frame = interp.frame.cloneDetached(cancel)
	}
	f := interp.frame
	interp.mutex.Unlock()

	f.mutex.Lock()
	interp.resizeFrameTo(f)
	f.setrunid(interp.runid())
	f.done = reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(cancel)}
	f.cancel = cancel
	f.mutex.Unlock()

	// Secondary ownedGC trigger: the fence is already held exclusively here,
	// so a pending sweep runs inline against the Locked body instead of
	// upgrading later — the fence must never be (re)acquired while funcMu is
	// held, and the registry insert sites only arm under funcMu. The same
	// inFlight CAS guards against the exec-step trigger; a panic still clears
	// inFlight and the enclosing defer releases the fence. The frameDrains
	// check is exact under the held fence: a drain starting underneath would
	// have to take the fence first.
	if interp.ownedGCPending.Load() && interp.ownedGCInFlight.CompareAndSwap(false, true) {
		func() {
			defer interp.ownedGCInFlight.Store(false)
			if interp.frameDrains.Load() != 0 {
				return
			}
			interp.ownedGCPending.Store(false)
			interp.ownedGCSweepLocked()
		}()
	}
	return f, nil
}

// ExecuteWithContext executes compiled Go code.
func (interp *Interpreter) ExecuteWithContext(ctx context.Context, p *Program) (res reflect.Value, err error) {
	if err := ctx.Err(); err != nil {
		return reflect.Value{}, err
	}
	release, err := interp.acquireExecutionWithContext(ctx)
	if err != nil {
		return reflect.Value{}, err
	}
	defer release()
	// The context controls this execution only. Keep a successful execution
	// owner open so interpreted functions in the result remain callable.
	runDone := make(chan struct{})
	publication := newExecutionPublication()

	done := make(chan struct{})
	go func() {
		defer close(done)
		res, err = interp.executeWithPublication(p, runDone, publication)
	}()

	select {
	case <-ctx.Done():
		close(runDone)
		publication.decision <- false
		return reflect.Value{}, ctx.Err()
	case <-publication.ready:
		if contextErr := ctx.Err(); contextErr != nil {
			close(runDone)
			publication.decision <- false
			return reflect.Value{}, contextErr
		}
		publication.decision <- true
		<-done
	case <-done:
		if contextErr := ctx.Err(); contextErr != nil {
			return reflect.Value{}, contextErr
		}
	}
	return res, err
}
