package vm

// TypeCache replaces the package-level sync.Map caches of AOT copies
// (cmd/aotgen rewrites their declarations): encoding/json's encoderCache and
// fieldCache, encoding/xml's tinfoMap, encoding/gob's userTypeCache... Those
// caches are indexed by reflect.Type, and in an AOT copy the descriptor of an
// interpreted type belongs to one machine: kept in a global map, it would
// keep the whole plugin (program, types, values) alive after the plugin is
// unloaded, for every type ever encoded. A TypeCache stores the entries of
// interpreted types in the type descriptor itself, so they live and die with
// their machine; other keys (native types, strings) go to a shared map, as
// before.

import "sync"

// TypeCache has the methods of sync.Map that the AOT copies use.
type TypeCache struct {
	shared sync.Map
}

// vmTypeKey returns the VM type descriptor key k stands for, or nil.
func vmTypeKey(k any) *vtype {
	if vt, ok := k.(*vtype); ok && vt != nil && vt.t != nil {
		return vt
	}
	return nil
}

func (c *TypeCache) Load(k any) (any, bool) {
	if vt := vmTypeKey(k); vt != nil {
		return vt.caches.Load(c)
	}
	return c.shared.Load(k)
}

func (c *TypeCache) Store(k, v any) {
	if vt := vmTypeKey(k); vt != nil {
		vt.caches.Store(c, v)
		return
	}
	c.shared.Store(k, v)
}

func (c *TypeCache) LoadOrStore(k, v any) (any, bool) {
	if vt := vmTypeKey(k); vt != nil {
		return vt.caches.LoadOrStore(c, v)
	}
	return c.shared.LoadOrStore(k, v)
}

func (c *TypeCache) LoadAndDelete(k any) (any, bool) {
	if vt := vmTypeKey(k); vt != nil {
		return vt.caches.LoadAndDelete(c)
	}
	return c.shared.LoadAndDelete(k)
}

func (c *TypeCache) Delete(k any) {
	if vt := vmTypeKey(k); vt != nil {
		vt.caches.Delete(c)
		return
	}
	c.shared.Delete(k)
}

// Range visits the shared entries only: the entries of interpreted types
// are reachable from their types alone.
func (c *TypeCache) Range(f func(k, v any) bool) { c.shared.Range(f) }

// TypeMap replaces the package-level maps indexed by reflect.Type of AOT
// copies (cmd/aotgen rewrites their declarations and uses): gopkg.in/yaml.v3's
// structMap, the field caches of toml and sigs.k8s.io/yaml, encoding/gob's
// types. As with TypeCache, the entries of interpreted types live in their
// descriptor; the others in a plain map, which the copy protects with its
// own lock as it did the original map.
type TypeMap[V any] struct {
	shared map[any]V
	init   bool
}

// Get is m[k].
func (m *TypeMap[V]) Get(k any) V {
	v, _ := m.Lookup(k)
	return v
}

// Lookup is v, ok := m[k].
func (m *TypeMap[V]) Lookup(k any) (V, bool) {
	if vt := vmTypeKey(k); vt != nil {
		if x, ok := vt.caches.Load(m); ok {
			return x.(V), true
		}
		var zero V
		return zero, false
	}
	v, ok := m.shared[k]
	return v, ok
}

// Set is m[k] = v.
func (m *TypeMap[V]) Set(k any, v V) {
	m.init = true
	if vt := vmTypeKey(k); vt != nil {
		vt.caches.Store(m, v)
		return
	}
	if m.shared == nil {
		m.shared = map[any]V{}
	}
	m.shared[k] = v
}

// Delete is delete(m, k).
func (m *TypeMap[V]) Delete(k any) {
	if vt := vmTypeKey(k); vt != nil {
		vt.caches.Delete(m)
		return
	}
	delete(m.shared, k)
}

// Init is m = make(...): the map is no longer nil.
func (m *TypeMap[V]) Init() { m.init = true }

// IsNil is m == nil.
func (m *TypeMap[V]) IsNil() bool { return !m.init }
