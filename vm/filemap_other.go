//go:build !unix

package vm

import "reflect"

// fileMapFuncs: golang.org/x/sys/unix exists only on unix systems.
func (m *Machine) fileMapFuncs(*Package, map[string]reflect.Value) {}
