// Command yhost loads a Traefik plugin with yaegi v0.16.1 exactly as Traefik
// does (pkg/plugins/middlewareyaegi.go and builder.go: a GOPATH holding the
// plugin module under src/<module>, stdlib symbols, unsafe and syscall only
// with useUnsafe, Eval of the import, then <basePkg>.New and .CreateConfig)
// and runs the catalog scenario of driver.go.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
	"github.com/traefik/yaegi/stdlib/syscall"
	"github.com/traefik/yaegi/stdlib/unsafe"
	"github.com/traefik/yaegi/v2/compare/catalog/yhost/plugins"
	"golang.org/x/mod/modfile"
)

type yaegiEngine struct {
	fnNew, fnCreateConfig reflect.Value
	i                     *interp.Interpreter // for the provider wrapper (provider.go)
	imp, basePkg          string
}

func (e *yaegiEngine) load(dir, imp, basePkg string, useUnsafe bool) error {
	gomod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return err
	}
	mod := modfile.ModulePath(gomod)
	gp, err := os.MkdirTemp("", "yhost-gopath-")
	if err != nil {
		return err
	}
	dst := filepath.Join(gp, "src", filepath.FromSlash(mod))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	abs, _ := filepath.Abs(dir)
	if err := os.Symlink(abs, dst); err != nil {
		return err
	}
	i := interp.New(interp.Options{GoPath: gp, Env: os.Environ(), Stdout: os.Stderr, Stderr: os.Stderr})
	if err := i.Use(stdlib.Symbols); err != nil {
		return err
	}
	if err := i.Use(plugins.Symbols()); err != nil {
		return err
	}
	if useUnsafe {
		if err := i.Use(unsafe.Symbols); err != nil {
			return err
		}
		if err := i.Use(syscall.Symbols); err != nil {
			return err
		}
	}
	if _, err := i.Eval(fmt.Sprintf(`import "%s"`, imp)); err != nil {
		return err
	}
	if basePkg == "" {
		basePkg = strings.ReplaceAll(path.Base(imp), "-", "_")
	}
	e.i, e.imp, e.basePkg = i, imp, basePkg
	if e.fnNew, err = i.Eval(basePkg + `.New`); err != nil {
		return err
	}
	e.fnCreateConfig, err = i.Eval(basePkg + `.CreateConfig`)
	return err
}

func (e *yaegiEngine) createConfig(testData map[string]any) (any, error) {
	res := e.fnCreateConfig.Call(nil)
	if len(res) != 1 {
		return nil, fmt.Errorf("CreateConfig returns %d values", len(res))
	}
	if err := traefikDecode(res[0].Interface(), testData); err != nil {
		return nil, err
	}
	return res[0], nil
}

func (e *yaegiEngine) newHandler(ctx context.Context, next http.Handler, cfg any, name string) (http.Handler, error) {
	res := e.fnNew.Call([]reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(next), cfg.(reflect.Value), reflect.ValueOf(name)})
	if len(res) > 1 && res[1].Interface() != nil {
		err, ok := res[1].Interface().(error)
		if !ok {
			return nil, fmt.Errorf("invalid error type: %T", res[1].Interface())
		}
		return nil, err
	}
	h, ok := res[0].Interface().(http.Handler)
	if !ok {
		return nil, fmt.Errorf("invalid handler type: %T", res[0].Interface())
	}
	return h, nil
}

func main() {
	newEngine = func() engine { return &yaegiEngine{} }
	runMain(&yaegiEngine{})
}
