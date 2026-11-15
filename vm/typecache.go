package vm

// TypeCache replaces the package-level sync.Map caches of AOT copies
// (cmd/aotgen rewrites their declarations): encoding/json's encoderCache and
// fieldCache, encoding/xml's tinfoMap, encoding/gob's userTypeCache... Those
// caches are indexed by reflect.Type, and in an AOT copy the descriptor of an
// interpreted type belongs to one machine: kept in a global map, it would
// keep the whole plugin (program, types, values) alive after the plugin is
// unloaded, for every type ever encoded. A TypeCache stores the entries of
// interpreted types in the type descriptor itself, so they live and die with
// their machine; native types go to a shared map, as before.
//
// A string is a name, not a type: encoding/gob's nameToConcreteType
// (name -> type) and concreteTypeToName (type -> name) are the registry of
// gob.RegisterName. An entry with a string key or a string value stored
// while a plugin runs (curMachine) goes to a table of that machine, and a
// load reads that table first, then the shared one (the names that gob
// registers at the init of the process). Both directions must be per
// machine, also for native types such as map[string]any: else plugin B
// finds the name that plugin A gave to the type (Encode works, Decode
// fails), and cannot give the type another name. Once a machine stored a
// string value in a cache, a load of a native type key reads the table of
// the machine too (names). A plugin that called gob.Register in its init
// could not be loaded twice ("gob: registering duplicate types for ...:
// p.T != p.T": the second load has other type descriptors), and the names
// of a plugin blocked the same names in another plugin. The entries of a
// machine go away with it. On a goroutine without a machine (started by AOT
// code), a load reads the shared map, then the last machine that stored the
// key (lastLoad).

import (
	"sync"
	"sync/atomic"
	"weak"
)

// TypeCache has the methods of sync.Map that the AOT copies use.
type TypeCache struct {
	shared sync.Map
	last   sync.Map    // name key -> weak.Pointer[Machine]: the last machine that stored it
	names  atomic.Bool // a machine stored an entry with a string value (type -> name)
}

// vmTypeKey returns the VM type descriptor key k stands for, or nil.
func vmTypeKey(k any) *vtype {
	if vt, ok := k.(*vtype); ok && vt != nil && vt.t != nil {
		return vt
	}
	return nil
}

// isName reports whether the entry k -> v is a registry entry: its key or
// its value is a name (a string).
func isName(k, v any) bool {
	if _, ok := k.(string); ok {
		return true
	}
	_, ok := v.(string)
	return ok
}

// nameKey reports whether a load or a delete of k (not an interpreted
// type) may find an entry in the table of a machine: k is a name, or a
// machine stored a type -> name entry in c.
func (c *TypeCache) nameKey(k any) bool {
	if _, ok := k.(string); ok {
		return true
	}
	return c.names.Load()
}

// table returns the table of m for c (create: make it when it does not
// exist), or nil.
func (c *TypeCache) table(m *Machine, create bool) *sync.Map {
	if t, ok := m.typeCaches.Load(c); ok {
		return t.(*sync.Map)
	}
	if !create {
		return nil
	}
	t, _ := m.typeCaches.LoadOrStore(c, &sync.Map{})
	return t.(*sync.Map)
}

// storeTable returns the table of the machine of the goroutine for the
// entry k -> v, and notes the machine as the last one that stored k, or
// nil when the entry goes to the shared map.
func (c *TypeCache) storeTable(k, v any) *sync.Map {
	if !isName(k, v) {
		return nil
	}
	m := curMachine()
	if m == nil {
		return nil
	}
	if _, ok := v.(string); ok {
		c.names.Store(true)
	}
	c.last.Store(k, weak.Make(m))
	return c.table(m, true)
}

// lastLoad is Load of the name key k on a goroutine without a machine
// (curMachine nil): the entry of the shared map, else the entry of the last
// machine that stored k, while it lives. The goroutines that AOT code
// starts itself have no machine: the server of the AOT copy of net/rpc
// (rpc.Accept) decodes the gob values of the plugin there.
func (c *TypeCache) lastLoad(k any) (any, bool) {
	if v, ok := c.shared.Load(k); ok {
		return v, true
	}
	w, ok := c.last.Load(k)
	if !ok {
		return nil, false
	}
	m := w.(weak.Pointer[Machine]).Value()
	if m == nil {
		return nil, false
	}
	if t := c.table(m, false); t != nil {
		return t.Load(k)
	}
	return nil, false
}

func (c *TypeCache) Load(k any) (any, bool) {
	if vt := vmTypeKey(k); vt != nil {
		return vt.caches.Load(c)
	}
	if !c.nameKey(k) {
		return c.shared.Load(k)
	}
	m := curMachine()
	if m == nil {
		return c.lastLoad(k)
	}
	if t := c.table(m, false); t != nil {
		if v, ok := t.Load(k); ok {
			return v, true
		}
	}
	return c.shared.Load(k)
}

func (c *TypeCache) Store(k, v any) {
	if vt := vmTypeKey(k); vt != nil {
		vt.caches.Store(c, v)
		return
	}
	if t := c.storeTable(k, v); t != nil {
		t.Store(k, v)
		return
	}
	c.shared.Store(k, v)
}

func (c *TypeCache) LoadOrStore(k, v any) (any, bool) {
	if vt := vmTypeKey(k); vt != nil {
		return vt.caches.LoadOrStore(c, v)
	}
	if old, ok := c.shared.Load(k); ok {
		return old, true
	}
	if t := c.storeTable(k, v); t != nil {
		return t.LoadOrStore(k, v)
	}
	return c.shared.LoadOrStore(k, v)
}

// deleteTable returns the table of the machine of the goroutine for a
// delete of k, a table with no entries when the machine has none, or nil
// when k goes to the shared map. A machine never deletes a name of the
// shared map.
func (c *TypeCache) deleteTable(k any) *sync.Map {
	if !c.nameKey(k) {
		return nil
	}
	m := curMachine()
	if m == nil {
		return nil
	}
	if t := c.table(m, false); t != nil {
		return t
	}
	return &noEntries
}

// noEntries is the table of a machine that stored no entry (never written).
var noEntries sync.Map

func (c *TypeCache) LoadAndDelete(k any) (any, bool) {
	if vt := vmTypeKey(k); vt != nil {
		return vt.caches.LoadAndDelete(c)
	}
	if t := c.deleteTable(k); t != nil {
		return t.LoadAndDelete(k)
	}
	return c.shared.LoadAndDelete(k)
}

func (c *TypeCache) Delete(k any) {
	if vt := vmTypeKey(k); vt != nil {
		vt.caches.Delete(c)
		return
	}
	if t := c.deleteTable(k); t != nil {
		t.Delete(k)
		return
	}
	c.shared.Delete(k)
}

// Range visits the shared entries only: the entries of interpreted types
// are reachable from their types alone, and the entries of a machine
// from the machine.
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
