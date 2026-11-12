package main

// Interpreted values in native containers (sync.Map, atomic.Value), in
// interpreted ones (container/list), and runtime.SetFinalizer on an
// interpreted pointer. The import of a package of this module makes the VM
// load everything from source, where the hooks must still apply.

import (
	"container/list"
	"fmt"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/traefik/yaegi/v2/vm/testdata/subplugin/inner"
)

type Session struct{ User string }
type hostKey struct{ Host string }
type pathKey struct{ Host string } // same layout as hostKey: distinct keys

type cache struct{ items map[string]int }
type Cache struct{ *cache } // embedded unexported pointer, as in go-cache

func main() {
	fmt.Println(inner.Tag)

	var m sync.Map
	m.Store(hostKey{"a"}, Session{"x"})
	m.Store(pathKey{"a"}, &Session{"y"})
	m.Store("plain", 1)
	tags := []string{"t"}
	m.Store(hostKey{"tags"}, tags)
	v, ok := m.Load(hostKey{"a"})
	fmt.Println(v.(Session).User, ok)
	p, _ := m.Load(pathKey{"a"})
	p.(*Session).User = "changed" // the stored pointer is the program's own
	p2, _ := m.Load(pathKey{"a"})
	fmt.Println(p2.(*Session).User)
	t, _ := m.Load(hostKey{"tags"})
	t.([]string)[0] = "shared"
	fmt.Println(tags[0])
	act, loaded := m.LoadOrStore(hostKey{"a"}, Session{"z"})
	fmt.Println(act.(Session).User, loaded)
	fmt.Println(m.CompareAndSwap(hostKey{"a"}, Session{"x"}, Session{"w"}), m.CompareAndSwap(hostKey{"a"}, Session{"x"}, Session{"v"}))
	var keys []string
	m.Range(func(k, v any) bool {
		switch k := k.(type) {
		case hostKey:
			keys = append(keys, "hostKey "+k.Host)
		case pathKey:
			keys = append(keys, "pathKey "+k.Host)
		case string:
			keys = append(keys, "string "+k)
		}
		return true
	})
	sort.Strings(keys)
	fmt.Println(keys)

	var av atomic.Value
	av.Store(Session{"one"})
	fmt.Println(av.Load().(Session).User)
	old := av.Swap(Session{"two"})
	fmt.Println(old.(Session).User, av.Load().(Session).User, av.CompareAndSwap(Session{"two"}, Session{"three"}), av.Load())

	l := list.New()
	items := map[any]*list.Element{}
	for _, h := range []string{"a", "b"} {
		items[hostKey{h}] = l.PushFront(hostKey{h})
	}
	back := l.Back()
	k := l.Remove(back)
	delete(items, k)
	_, still := items[hostKey{"a"}]
	fmt.Println(k == hostKey{"a"}, still, len(items), l.Len())

	c := &Cache{&cache{items: map[string]int{"n": 1}}}
	runtime.SetFinalizer(c, func(c *Cache) { c.items = nil })
	runtime.SetFinalizer(c, nil)
	fmt.Println(c.items["n"])
}

// wrapped has an unexported interface field holding an interpreted error.
type wrapped struct{ err error }

type myErr struct{ msg string }

func (e myErr) Error() string { return e.msg }

func init() {
	var v atomic.Value
	v.Store(wrapped{myErr{"inner"}})
	w := v.Load().(wrapped)
	_, isMine := w.err.(myErr)
	fmt.Println(w.err.Error(), isMine)
}
