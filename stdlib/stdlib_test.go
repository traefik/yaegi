package stdlib

import (
	"path"
	"reflect"
	"strings"
	"testing"
)

// TestInterfaceWrappersPkgPath checks that each interface wrapper can be found
// from the package path reported by reflect. This is not the case when an
// interface is an alias of a type defined in another package.
func TestInterfaceWrappersPkgPath(t *testing.T) {
	// Index the wrappers by the package path used by the interpreter.
	wrappers := map[string]map[string]reflect.Value{}
	for k, syms := range Symbols {
		wrappers[path.Dir(k)] = syms
	}

	for k, syms := range Symbols {
		for name, v := range syms {
			if !strings.HasPrefix(name, "_") || v.Kind() != reflect.Pointer {
				continue
			}

			it, ok := syms[name[1:]]
			if !ok || it.Kind() != reflect.Pointer || it.Type().Elem().Kind() != reflect.Interface {
				continue
			}

			typ := it.Type().Elem()
			if _, ok := wrappers[typ.PkgPath()]["_"+typ.Name()]; !ok {
				t.Errorf("%s.%s: no wrapper registered for %s.%s", path.Dir(k), name[1:], typ.PkgPath(), typ.Name())
			}
		}
	}
}
