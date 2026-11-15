package vm

// The profiles of runtime/pprof of a machine (hostregs.go).
//
// pprof.NewProfile adds the profile to a table of the process. A plugin
// with `var prof = pprof.NewProfile("x")` failed its second load ("pprof:
// NewProfile name already in use"), so a reload of the plugin broke it; two
// plugins that use the same library could not be loaded side by side; and
// a plugin could take a name before the host, which then panicked at its
// own NewProfile. pprof.Lookup("goroutine").WriteTo(w, 2) gave the plugin
// the stacks of every goroutine of the host, with their arguments.
//
// A machine without the real standard library (not m.realStd) has its own
// table of profiles:
//
//   - NewProfile makes a profile that is not in the table of the process,
//     and adds it to the machine's table. It panics as Go does for an empty
//     name, a name already in the machine's table, and the names of the
//     built-in profiles (goroutine, heap...), which always exist in Go.
//   - Lookup and Profiles read the machine's table and the built-in
//     profiles, not the profiles that the host made. The built-in profiles
//     goroutine and goroutineleak are stand-ins of the machine, with no
//     entry: the real ones write the stacks of every goroutine of the host.
//     The other built-in profiles (heap, allocs, block, mutex,
//     threadcreate) are the host's: they are snapshots, as
//     pprof.WriteHeapProfile, which plugins use (lukaszraczylo/traefikoidc
//     writes the heap profile, and the goroutine profile with debug 0).
//
// The methods of the profiles (Add, Remove, Count, WriteTo, Name) are the
// methods of pprof. The profiles of a machine go away with it.
//
// A *pprof.Profile out of the table of the process cannot be made with the
// API of pprof: newProfile sets the unexported fields name and m with
// unsafe, after a check of their names and types with reflect (a new Go
// release that changes them makes NewProfile panic in a plugin, and
// TestPluginRegistriesReload fail).

import (
	"reflect"
	"runtime/pprof"
	"slices"
	"strings"
	"sync"
	"unsafe"
)

// pprofBuiltin are the names of the built-in profiles of pprof; true for
// the ones that the machine replaces with a stand-in.
var pprofBuiltin = map[string]bool{
	"goroutine": true, "goroutineleak": true,
	"threadcreate": false, "heap": false, "allocs": false, "block": false, "mutex": false,
}

// pprofState is the table of profiles of a machine.
type pprofState struct {
	mu       sync.Mutex
	profiles map[string]*pprof.Profile // NewProfile
	standIns map[string]*pprof.Profile // the stand-ins of pprofBuiltin
}

// lookup is Lookup on the table of the machine (under st.mu).
func (st *pprofState) lookup(name string) *pprof.Profile {
	if p := st.profiles[name]; p != nil {
		return p
	}
	standIn, builtin := pprofBuiltin[name]
	switch {
	case !builtin:
		return nil
	case !standIn:
		return pprof.Lookup(name)
	}
	p := st.standIns[name]
	if p == nil {
		if name == "goroutineleak" && pprof.Lookup(name) == nil {
			return nil // not in this Go (GOEXPERIMENT=goroutineleakprofile)
		}
		if st.standIns == nil {
			st.standIns = map[string]*pprof.Profile{}
		}
		p = newProfile(name)
		st.standIns[name] = p
	}
	return p
}

// pprofFields are the offsets of the unexported fields name and m of
// pprof.Profile (ok false when their types are not the expected ones: a
// new Go release changed the struct).
var pprofFields = sync.OnceValues(func() ([2]uintptr, bool) {
	rt := reflect.TypeFor[pprof.Profile]()
	name, ok1 := rt.FieldByName("name")
	m, ok2 := rt.FieldByName("m")
	if !ok1 || !ok2 || name.Type != reflect.TypeFor[string]() || m.Type != reflect.TypeFor[map[any][]uintptr]() {
		return [2]uintptr{}, false
	}
	return [2]uintptr{name.Offset, m.Offset}, true
})

// newProfile returns a profile named name that is not in the table of the
// process, as pprof.NewProfile makes it.
func newProfile(name string) *pprof.Profile {
	offs, ok := pprofFields()
	if !ok {
		panic("vm: pprof.NewProfile in a plugin: the fields of pprof.Profile are not the expected ones (vm/hostpprof.go)")
	}
	p := new(pprof.Profile)
	*(*string)(unsafe.Add(unsafe.Pointer(p), offs[0])) = name
	*(*map[any][]uintptr)(unsafe.Add(unsafe.Pointer(p), offs[1])) = map[any][]uintptr{}
	return p
}

func (m *Machine) pprofFunc(name string) any {
	st := &m.hostRegs().pprof
	switch name {
	case "NewProfile":
		return func(name string) *pprof.Profile {
			if name == "" {
				panic("pprof: NewProfile with empty name")
			}
			st.mu.Lock()
			defer st.mu.Unlock()
			if _, builtin := pprofBuiltin[name]; builtin || st.profiles[name] != nil {
				panic("pprof: NewProfile name already in use: " + name)
			}
			if st.profiles == nil {
				st.profiles = map[string]*pprof.Profile{}
			}
			p := newProfile(name)
			st.profiles[name] = p
			return p
		}
	case "Lookup":
		return func(name string) *pprof.Profile {
			st.mu.Lock()
			defer st.mu.Unlock()
			return st.lookup(name)
		}
	case "Profiles":
		return func() []*pprof.Profile {
			st.mu.Lock()
			all := make([]*pprof.Profile, 0, len(st.profiles)+len(pprofBuiltin))
			for _, p := range st.profiles {
				all = append(all, p)
			}
			for name := range pprofBuiltin {
				if p := st.lookup(name); p != nil {
					all = append(all, p)
				}
			}
			st.mu.Unlock()
			slices.SortFunc(all, func(a, b *pprof.Profile) int { return strings.Compare(a.Name(), b.Name()) })
			return all
		}
	}
	return nil
}
