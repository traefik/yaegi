package main

import (
	"bufio"
	"bytes"
	"context"
	"go/build"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

var (
	root     string // repository root
	yaegiBin string // yaegi binary built by TestMain
	wsDir    string // workspace copy of tests/yaegi/_test
)

func TestMain(m *testing.M) {
	os.Exit(testMain(m))
}

func testMain(m *testing.M) int {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	if root, err = FindRoot(wd); err != nil {
		panic(err)
	}
	tmp, err := os.MkdirTemp("", "yaegi-test-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmp)
	yaegiBin = filepath.Join(tmp, "yaegi"+exeSuffix)
	if err := BuildVM(root, yaegiBin); err != nil {
		panic(err)
	}
	ws, cleanup, err := PrepareWorkspace(filepath.Join(root, "tests", "yaegi", "_test"), "1.22")
	if err != nil {
		panic(err)
	}
	defer cleanup()
	wsDir = ws
	return m.Run()
}

// testdataCase is one way to run a testdata program.
type testdataCase struct {
	flags   []string // yaegi flags
	goFlags []string // `go run` flags
	env     []string // environment of both runs (GOEXPERIMENT=...)
	// diverge: known divergence from gc (the reflect wall); only the exit
	// status is checked, and the outputs must differ.
	diverge string
	// maskAddrs: the program prints addresses with print or println, and
	// they differ between runs. Both outputs get each non-zero hex number
	// replaced by "addr" before the comparison; 0x0 stays, so a nil value
	// and a non-nil value still print differently.
	maskAddrs bool
}

var hexAddr = regexp.MustCompile(`0x[0-9a-f]+`)

// maskAddrs replaces the non-zero hex numbers of out by "addr".
func maskAddrs(out []byte) []byte {
	return hexAddr.ReplaceAllFunc(out, func(h []byte) []byte {
		if string(h) == "0x0" {
			return h
		}
		return []byte("addr")
	})
}

// testdataCases is the per-file configuration. Every file is run at least
// once; a file listed here runs each of its variants instead of the default.
// TestTestdata fails if a top-level testdata/*.go file (or the tags dir) is
// missing from the run, so a new program cannot be forgotten, and if a key
// here no longer exists on disk.
var testdataCases = map[string][]testdataCase{
	"contextvals.go": {{}, {flags: []string{"-aot"}}},
	// the init order of gc over its graph of init tasks (vm/initorder.go)
	"initregistry.go": {{}, {flags: []string{"-aot"}}},
	"initleaf.go":     {{}, {flags: []string{"-aot"}}},
	// allocations counted by the program: the AOT copies and the opaque
	// handles take other paths at the native boundary
	"nativecallallocs.go": allModes,
	// a variable of plain data in real bytes because the address of a
	// field or an element goes to native code: fmt, json and flag in every
	// mode (native, the AOT copies, opaque handles)
	"plainaddrs.go": allModes,
	// unsafe header views on 32 goroutines: default mode, and a plugin
	// with useUnsafe (-restricted -unsafe), as in Traefik.
	"unsafegoroutines.go": {{}, {flags: []string{"-restricted", "-unsafe"}}},
	// a data pointer kept in a struct after the ring of regions forgot it
	// (protobuf's protoreflect.Value under cel-go)
	"unsafeextent.go": {{}, {flags: []string{"-restricted", "-unsafe"}}},
	// cap of []byte("constant") and the Cap of a header view of it: default
	// mode, the AOT copies, opaque handles, and a plugin with useUnsafe
	"constbytescap.go": {{}, {flags: []string{"-aot"}}, {flags: []string{"-opaque"}}, {flags: []string{"-restricted", "-unsafe"}}},
	// a *reflect.Value in a sync.Pool (fxamacker/cbor): the AOT shadows
	"reflectpoolshadow.go": {{}, {flags: []string{"-aot"}}},
	// native code that appends in place into the capacity of a slice of
	// cells (hash.Sum(id[:0]) of wazero)
	"nativeappendtail.go": {{}, {flags: []string{"-aot"}}},
	// functions linked to the runtime (wazero, x/sys/cpu)
	"linkruntime.go": {{}, {flags: []string{"-restricted", "-unsafe"}}},
	// syscall.RawConn as gRPC uses it: dialer Control, and syscall.Read of
	// the lent descriptor in RawConn.Read (a plugin with useUnsafe)
	"rawconnread.go": {{}, {flags: []string{"-restricted", "-unsafe"}}, {flags: []string{"-restricted", "-unsafe", "-aot"}}},
	// syscall.Statfs in the syscall subset of useUnsafe (prometheus/procfs)
	"statfs.go": {{}, {flags: []string{"-restricted", "-unsafe"}}, {flags: []string{"-restricted", "-unsafe", "-aot"}}},
	// syscall.SetsockoptInt and GetsockoptInt on the socket that a RawConn
	// lends, for the allowed options (gRPC sets TCP_USER_TIMEOUT on linux)
	"sockopt.go": {{}, {flags: []string{"-restricted", "-unsafe"}}, {flags: []string{"-restricted", "-unsafe", "-aot"}}},
	// the address of a variable given to syscall.Syscall as a uintptr
	// (real memory for the kernel): native struct, struct of the program,
	// field, array, integer
	"syscalladdr.go": allModes,
	// a cell address as a uintptr given to sync/atomic (a number, not
	// memory): not refused, and a plugin with useUnsafe can do it
	"atomicuintptr.go": {{}, {flags: []string{"-aot"}}, {flags: []string{"-opaque"}}, {flags: []string{"-restricted", "-unsafe"}}},
	// os/exec alone in a restricted program (Options.Exec = ExecReal)
	"execreal.go":   {{}, {flags: []string{"-restricted", "-exec", "real"}}},
	"reflectown.go": {{flags: []string{"-aot"}}},
	// error fields of a program struct printed by fmt: an unexported one
	// is printed as a struct, in default mode too (aot containers)
	"errfields.go": {{}, {flags: []string{"-aot"}}, {flags: []string{"-restricted"}}},
	// the 101 body of a program RoundTripper with Write is an
	// io.ReadWriteCloser for httputil.ReverseProxy (vm/optwrap.go)
	"upgradebody.go": {{}, {flags: []string{"-aot"}}, {flags: []string{"-restricted"}}},
	// io.Copy uses WriteTo and ReadFrom of program types that have more
	// methods than a family of vm/optwrap.go
	"copywriteto.go": {{}, {flags: []string{"-aot"}}, {flags: []string{"-restricted"}}},
	// int, uint and uintptr have the size of the platform (32 bits on 386
	// and arm): default mode, AOT copies (twin reflect), and math/bits and
	// encoding/json interpreted from source.
	"intsize.go": {{}, {flags: []string{"-aot"}}, {flags: []string{"-src=math/bits,encoding/json"}}},
	// the sandbox check of the twin path: Set of an unexported field of a
	// host object panics with the import restrictions too
	"reflecthost.go": {{}, {flags: []string{"-restricted"}}},
	// a ReverseProxy with a nil Transport uses the machine's DefaultTransport
	// in restricted mode, as the DefaultClient does
	"reqgetbody.go": {{}, {flags: []string{"-restricted"}}, {flags: []string{"-restricted", "-aot"}}},
	// encoding/json/v2 and jsontext (Go 1.27): the AOT copies in default
	// mode, their generic functions (aotgeneric.go), and the native
	// instances of the generic functions of jsontext without the copy
	"jsontextgeneric.go": append(allModes, testdataCase{flags: []string{"-restricted"}}, testdataCase{flags: []string{"-restricted", "-aot"}},
		testdataCase{flags: []string{"-mode", "encoding/json/jsontext=native"}}),
	"jsonv2generic.go": append(allModes, testdataCase{flags: []string{"-restricted"}}, testdataCase{flags: []string{"-restricted", "-aot"}}),
	"jsonv2api.go":     append(allModes, testdataCase{flags: []string{"-restricted"}}, testdataCase{flags: []string{"-restricted", "-aot"}}),
	// The VM reflect (local type names, MakeFunc...): the program's own
	// reflect calls in default mode, and the AOT copies.
	"localtypes.go":   {{}, {flags: []string{"-aot"}}},
	"reflectfuncs.go": {{}, {flags: []string{"-aot"}}},
	"recoverwrap.go":  {{}, {flags: []string{"-aot"}}},
	// reflect.Append of a []byte when the elements do not all fit
	"reflectappendbytes.go": {{}, {flags: []string{"-aot"}}},
	// fmt and encoding/xml with interpreted types: their AOT copies called
	// from default mode (vm/aotroute.go), and AOT mode.
	"aotroute.go": {{}, {flags: []string{"-aot"}}},
	// the errors of json routed to its AOT copy name the interpreted
	// types, and a value with a json.RawMessage goes to the copy too
	"jsonrouteerrors.go": allModes,
	// a program that imports text/template, html/template, log/slog,
	// encoding/gob or encoding/xml runs the standard library as in AOT
	// mode (stdAOTModes, vm/aot.go)
	"stdaottext.go": allModes,
	"stdaothtml.go": allModes,
	"stdaotslog.go": allModes,
	"stdaotgob.go":  allModes,
	"stdaotxml.go":  allModes,
	"xmlnative.go":  {{}, {flags: []string{"-aot"}}},
	"nanmixed.go":   {{}, {flags: []string{"-aot"}}},
	// a func, pointer or map read from a field of a native struct is a copy
	"nativefieldcopy.go": {{}, {flags: []string{"-aot"}}},
	// a struct read from a field of a native struct is a copy
	"nativestructcopy.go": {{}, {flags: []string{"-aot"}}},
	// a pointer to a field of a native struct in a cell sees the struct assigned
	"nativestructassign.go": {{}, {flags: []string{"-aot"}}},
	// maps and slices filled by native code through a pointer: native code
	// in default mode, AOT copies (and Unbox) in AOT mode.
	"nativeident.go": {{}, {flags: []string{"-aot"}}},
	// values equal by == but not the same (0 and -0, MyInt(1) and 1,
	// complex -0) in maps and slices written back after a native call.
	"nativezero.go": {{}, {flags: []string{"-aot"}}},
	// funcs and NaN map keys in maps and slices given to native code: a
	// read only call keeps them, changes made by native code are kept.
	"nativefunc.go": {{}, {flags: []string{"-aot"}}},
	// json.Unmarshal into a *json.Number or a *time.Duration: the shadow
	// of the cell is written back in default mode.
	"jsonnumberdst.go": allModes,
	// a json.Number of a Decoder (UseNumber) or of the program given back
	// to json.Marshal or an Encoder that run on the AOT copy is a number
	"jsonnumberroute.go": allModes,
	// interpreted channels given to native code in an interface: a native
	// stand-in (vm/chanstandin.go) in default mode, Unbox in AOT mode.
	// Not -opaque: json wraps the error of the MarshalJSON of the handle.
	"chaninany.go": {{}, {flags: []string{"-aot"}}},
	// a struct with an embedded unexported field (no native counterpart)
	// stored by native code (sync.Pool, sync.Map, a context).
	"embedunexported.go": {{}, {flags: []string{"-aot"}}},
	// values (not pointers) of generic, local and unexported plugin types
	// that native code gives back in an interface (sync.Pool's New,
	// atomic.Value, sync.Map, context...): they keep their type.
	"nativeanytypes.go": allModes,
	// zero-size values of plugin types and unnamed structs given back by
	// native code in an interface keep their type
	"zerosizeany.go": allModes,
	// writes through slices read from native memory (http.Header,
	// url.Values, tls.Config.NextProtos) reach the native array.
	// Decoding into a view: the twin reflect of the AOT copies (-aot), and
	// a shadow of the header for opaque handles (-opaque).
	"nativeslices.go":       {{}, {flags: []string{"-aot"}}, {flags: []string{"-aot", "-opaque"}}},
	"nativeslicesopaque.go": {{}, {flags: []string{"-aot"}}, {flags: []string{"-opaque"}}},
	// errors.Is, errors.As and errors.Unwrap with interpreted errors: VM
	// walks of the error chain in both modes (vm/errors.go).
	"errwrap.go":     {{}, {flags: []string{"-aot"}}},
	"errchain.go":    {{}, {flags: []string{"-aot"}}},
	"storage.go":     {{}, {flags: []string{"-aot"}}},
	"genericstd.go":  {{}, {flags: []string{"-aot"}}},
	"stdnew.go":      {{}, {flags: []string{"-aot"}}},
	"poolmethods.go": {{}, {flags: []string{"-aot"}}},
	// gob.Register of interpreted types: the native gob with the mirror
	// types (vm/gob.go), and the AOT copy.
	"gobregister.go": {{}, {flags: []string{"-aot"}}},
	"tags":           {{goFlags: []string{"-tags=dummy"}, flags: []string{"-yaegi-tags"}}}, // "// yaegi:tags dummy"
	"g3.go":          {{flags: []string{"-src", "slices,maps,iter"}}},
	"srcdemo.go":     {{flags: []string{"-opaque", "-src", "sort,slices,errors,fmt,strconv,internal/strconv"}}},
	"srcdemo2.go":    {{flags: []string{"-src", "sort,slices"}}},
	"printiface.go":  {{maskAddrs: true}},
	// GOEXPERIMENT=fieldtrack: on a cold build cache, the first `go run`
	// with a new experiment rebuilds the runtime and the standard library
	// (a few seconds more).
	"nointerface.go":         {{env: fieldTrack}, {flags: []string{"-opaque"}, env: fieldTrack}},
	"nointerface_reflect.go": {{flags: []string{"-opaque"}, env: fieldTrack}},
	"unique.go":              {{}, {flags: []string{"-aot"}}},
	// == on native struct values (netip.Addr, time.Time): the VM ==, the
	// twin reflect of the AOT copies, and opaque handles.
	"nativeeq.go": {{}, {flags: []string{"-aot"}}, {flags: []string{"-opaque"}}},
	// decoding into a map of structs with an unexported field: the native
	// map values have no address.
	"mapunexported.go": {{}, {flags: []string{"-aot"}}},
	// %T of unique.Handle[T] needs the VM reflect: in default mode, native
	// fmt only sees the layout of interpreted named types (wall.go).
	"uniquetype.go": {{flags: []string{"-aot"}}},
	// reflect.ValueOf(m).Clear() must clear the VM map itself: native
	// default-mode reflect works on a copy of the map (the reflect wall).
	"nanclear.go": {{flags: []string{"-aot"}}},
	// the interpreted reflect (Append, Grow, Copy) on VM slices; default
	// mode reflect works on native copies (the reflect wall)
	"reflgrow.go": {{flags: []string{"-aot"}}, {flags: []string{"-opaque"}}, {flags: []string{"-aot", "-opaque"}}},
	// json.Unmarshal into recursive types: native types unrolled along the
	// data in default mode (vm/recursive.go), the twin reflect in AOT mode.
	"jsonrec.go": {{}, {flags: []string{"-aot"}}},
	// decoding into a pointer to an interface that holds a pointer: the
	// pointed value is filled and keeps its identity.
	"jsonifaceptr.go": {{}, {flags: []string{"-aot"}}, {flags: []string{"-restricted", "-aot"}}},
	// json (and gob, xml) decoding into values whose type has methods:
	// the value is written back, and the destination of a decoder
	// is never a hand wrapper (vm/decodedst.go).
	"jsonptrmethods.go": {{}, {flags: []string{"-aot"}}},
	// fmt.Sscan family, binary.Read/Write and asn1 with values whose type
	// has methods (String, Error, fmt.Scanner): the arguments are never a
	// hand wrapper (vm/decodedst.go).
	"scandst.go": {{}, {flags: []string{"-aot"}}, {flags: []string{"-opaque"}}},
	// The same decoders called as func values (a struct field, a map, a
	// slice, an interface): the destination is never a hand wrapper either
	// (vm/decodedst.go, decodeDstPtrs).
	"decodefuncvalue.go": append(allModes, testdataCase{flags: []string{"-restricted"}}),
	// json.Marshal of values that contain themselves: the cycle check of
	// the AOT copy (vm.PtrKey), and the conversion to native data of maps
	// and slices in default mode (convGraph.enterComp). Both killed the
	// host with a fatal Go stack overflow. In default mode json goes to its
	// AOT copy (aotroute.go), which names the interpreted type as Go does.
	"jsoncycle.go":    {{}, {flags: []string{"-aot"}}, {flags: []string{"-restricted", "-aot"}}},
	"jsoncyclemap.go": {{}, {flags: []string{"-aot"}}, {flags: []string{"-restricted", "-aot"}}},
	// recursive named types that are not structs (type M map[string]M,
	// type T []T...): their native type cuts the parts that refer back to
	// the type (cutNamed in vm/native.go), and native values that contain
	// themselves come back into the VM with the same cycle (natSeen). Both
	// killed the host with a fatal Go stack overflow in default mode.
	"recnamed.go": {{}, {flags: []string{"-aot"}}, {flags: []string{"-restricted", "-budget", "100000000"}}},
	"natcycle.go": {{}, {flags: []string{"-aot"}}, {flags: []string{"-restricted", "-budget", "100000000"}}},
	// The same cycles inside a struct, converted back by fromNativeDeepC
	// (sync.Map, atomic.Value): a fatal Go stack overflow in every mode.
	"natcycledeep.go": {{}, {flags: []string{"-aot"}}, {flags: []string{"-restricted", "-budget", "100000000"}}},
	// Native printers (fmt, log) with opaque handles: %T printed
	// vm.vmOpaque for a []int.
	"opaquefmt.go": {{}, {flags: []string{"-opaque"}}, {flags: []string{"-aot"}}, {flags: []string{"-aot", "-opaque"}}},
	// A value nested a million times in interfaces, given to printers: a
	// fatal Go stack overflow of the host in every mode.
	"opaquefmtdeep.go": {{}, {flags: []string{"-aot"}}, {flags: []string{"-restricted", "-aot", "-budget", "100000000"}}},
	// The checks of the reflect setters (unaddressable values, unexported
	// fields, native structs in an interface) and the panic values of the
	// VM reflect.
	"reflectset.go": allModes,
	// a panic of a callback: native code (fmt, sort) recovers the value of
	// the program, not the panic value of the VM
	"callbackpanic.go": allModes,
	// reflect.DeepEqual with interpreted values: the twin reflect sees
	// them boxed in default mode too, not native copies
	"deepequaltwin.go": allModes,
	// Set and SetMapIndex into interfaces: the value is boxed, and a value
	// that is not assignable panics as in gc
	"reflectsetiface.go": allModes,
	// reflect of big plugin libraries (compare/complex): NewAt of an
	// unexported field, SetIterKey, Method.IsExported, flagEmbedRO, and
	// writes to a plugin struct that native code handed back.
	"reflectnewatfield.go":     allModes,
	"reflectsetiter.go":        allModes,
	"reflectmethodexported.go": allModes,
	"reflectembedro.go":        allModes,
	"nativefieldwrite.go":      allModes,
	"jsondecodeptrptr.go":      allModes,
	"nativeptrmethodiface.go":  allModes,
	// recover() after panic(nil) gives a *runtime.PanicNilError, or nil
	// with GODEBUG=panicnil=1.
	"panicnil.go": append(allModes, testdataCase{env: []string{"GODEBUG=panicnil=1"}}),

	// Regression net for language semantics and std interop: the same
	// program in default mode, AOT mode and with opaque handles, so that a
	// change in one path of the VM shows up.
	"loopvars.go":     allModes,
	"panicorder.go":   allModes,
	"chanselect.go":   append(allModes, testdataCase{flags: []string{"-restricted", "-budget", "100000000"}}),
	"methodsets.go":   allModes,
	"controlflow.go":  allModes,
	"numconv.go":      allModes,
	"stringsbytes.go": allModes,
	"aliasing.go":     allModes,
	"stdinterop.go":   allModes,
	"reflectmore.go":  allModes,
	"httpmw.go":       append(allModes, testdataCase{flags: []string{"-restricted"}}),
	"generics2.go":    allModes,
	// the paths of the performance work of TODO.md section 5 (fewer
	// allocations, trampolines, callbacks), against the general paths
	"natallocs.go": allModes,
	"nattramps.go": allModes,
	// with a budget: the callbacks take their steps from the account of
	// the call (nativeCallback)
	"natcallbacks.go": append(allModes, testdataCase{flags: []string{"-restricted", "-budget", "100000000"}}),
	// json and fmt see the methods of the types of inner fields (UnmarshalJSON,
	// Format...): in AOT mode, and in default mode through the AOT copies
	// (vm/aotroute.go). Not -opaque for jsonmore.go: json wraps the error of
	// the MarshalJSON of the handle.
	"jsonmore.go": {{}, {flags: []string{"-aot"}}, {flags: []string{"-restricted", "-aot"}}},
	"fmtverbs.go": {{}, {flags: []string{"-aot"}}, {flags: []string{"-opaque"}}},
	// json values that hold json.RawMessage or json.Number and types with
	// methods inside them: they go to the AOT copy of json in default mode.
	"jsonpkgtypes.go": allModes,
	// xml methods that Go does not call (flattened embedded structs, the
	// attribute and element methods, hidden fields) in a value with an
	// xml.Name: the check of vm/lostmethods.go must accept them (with xml
	// forced native too: a plugin that imports xml may run the standard
	// library as AOT copies, stdAOTModes). Not -opaque: xml of an opaque
	// handle gives <vmOpaque> (also on main).
	"xmlflatten.go": {{}, {flags: []string{"-aot"}}, {flags: []string{"-mode", "encoding/xml=native"}}},
}

// only64 are the testdata programs written for a 64-bit int: go run itself
// refuses them on 32-bit platforms (a constant that overflows int or
// uintptr), so there is nothing to compare with there. The value says why.
var only64 = map[string]string{
	"zeroslice.go":     "slices of zero-sized elements of length 1<<40",
	"unsafehdr.go":     "a slice header with a length of 1<<61",
	"runtimepanics.go": "an int index of 9876543210",
	"numfast.go":       "int and uint constants of 2^53+1 and 2^63+1",
	"syscalladdr.go":   "the layout of syscall.Timespec of 64-bit linux (an int64 Sec)",
}

// onlyGOOS are the testdata programs for one operating system: they use
// system calls of that system only, and go run cannot build them elsewhere.
var onlyGOOS = map[string]string{
	"syscalladdr.go": "linux",
}

// haveJSONv2: the toolchain of the test has encoding/json/v2 and jsontext
// (Go 1.27, or Go 1.26 with GOEXPERIMENT=jsonv2). The testdata programs
// that use them have the build constraint goexperiment.jsonv2: go run
// cannot build them without it, and TestTestdata skips them.
var haveJSONv2 = slices.Contains(build.Default.ToolTags, "goexperiment.jsonv2")

// allModes runs a program in default mode, AOT mode and with opaque handles.
var allModes = []testdataCase{{}, {flags: []string{"-aot"}}, {flags: []string{"-opaque"}}}

var fieldTrack = []string{"GOEXPERIMENT=fieldtrack"}

// caseEnv returns the environment of a run: os.Environ() with the entries
// of env. A GOEXPERIMENT entry is added to the GOEXPERIMENT already set (CI,
// developer), it does not replace it.
func caseEnv(env []string) []string {
	out := os.Environ()
	for _, e := range env {
		if v, ok := strings.CutPrefix(e, "GOEXPERIMENT="); ok {
			if old := os.Getenv("GOEXPERIMENT"); old != "" {
				e = "GOEXPERIMENT=" + old + "," + v
			}
		}
		out = append(out, e) // the last entry wins
	}
	return out
}

func TestCaseEnv(t *testing.T) {
	t.Setenv("GOEXPERIMENT", "jsonv2")
	env := caseEnv(fieldTrack)
	if got := env[len(env)-1]; got != "GOEXPERIMENT=jsonv2,fieldtrack" {
		t.Errorf("last entry = %q, want GOEXPERIMENT=jsonv2,fieldtrack", got)
	}
	t.Setenv("GOEXPERIMENT", "")
	env = caseEnv(fieldTrack)
	if got := env[len(env)-1]; got != "GOEXPERIMENT=fieldtrack" {
		t.Errorf("last entry = %q, want GOEXPERIMENT=fieldtrack", got)
	}
}

// discoverTestdata returns every top-level testdata entry to run: the *.go
// files and the "tags" directory.
func discoverTestdata(t *testing.T) []string {
	t.Helper()
	ents, err := os.ReadDir(filepath.Join(root, "testdata"))
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, e := range ents {
		if e.IsDir() {
			if e.Name() == "tags" {
				files = append(files, e.Name())
			}
			continue // "aot" is a separate suite (cmd/aotcheck)
		}
		if strings.HasSuffix(e.Name(), ".go") {
			files = append(files, e.Name())
		}
	}
	return files
}

// TestTestdata runs every program of testdata/ with the VM and with
// `go run`, and compares their combined output. It discovers the files so a
// new program is covered automatically.
func TestTestdata(t *testing.T) {
	files := discoverTestdata(t)
	present := map[string]bool{}
	for _, f := range files {
		present[f] = true
	}
	for name := range testdataCases {
		if !present[name] {
			t.Errorf("testdataCases has %q, which is not in testdata/", name)
		}
	}
	for name := range only64 {
		if !present[name] {
			t.Errorf("only64 has %q, which is not in testdata/", name)
		}
	}
	for name := range onlyGOOS {
		if !present[name] {
			t.Errorf("onlyGOOS has %q, which is not in testdata/", name)
		}
	}
	for _, f := range files {
		cases := testdataCases[f]
		if cases == nil {
			cases = []testdataCase{{}} // default: native mode, no flags
		}
		for _, c := range cases {
			name := f
			if len(c.flags) > 0 {
				name += " " + strings.Join(c.flags, "_")
			}
			t.Run(name, func(t *testing.T) {
				if why := only64[f]; why != "" && strconv.IntSize < 64 {
					t.Skipf("written for a 64-bit int (%s): go run refuses it on %s", why, runtime.GOARCH)
				}
				if goos := onlyGOOS[f]; goos != "" && goos != runtime.GOOS {
					t.Skipf("written for %s only", goos)
				}
				if ok, err := build.Default.MatchFile(filepath.Join(root, "testdata"), f); err == nil && !ok {
					t.Skip("excluded by its build constraint with this toolchain (encoding/json/v2 needs GOEXPERIMENT=jsonv2 with Go 1.26)")
				}
				t.Parallel()
				path := filepath.Join(root, "testdata", f)
				goCmd := hangCommand(t, "go", append(append([]string{"run"}, c.goFlags...), path)...)
				goCmd.Env = caseEnv(c.env)
				want, err := goCmd.CombinedOutput()
				if err != nil {
					t.Fatalf("go run: %v\n%s", err, want)
				}
				vmCmd := hangCommand(t, yaegiBin, append(append([]string{}, c.flags...), path)...)
				vmCmd.Env = caseEnv(c.env)
				got, err := vmCmd.CombinedOutput()
				if err != nil {
					t.Fatalf("yaegi: %v\n%s", err, got)
				}
				if c.maskAddrs {
					got, want = maskAddrs(got), maskAddrs(want)
				}
				if c.diverge != "" {
					if bytes.Equal(got, want) {
						t.Errorf("known divergence (%s) is gone: update the test", c.diverge)
					}
					return
				}
				if !bytes.Equal(got, want) {
					t.Errorf("got:\n%s\nwant (go run):\n%s", got, want)
				}
			})
		}
	}
}

// hangCommand returns the command of a testdata program. A run longer
// than 10 minutes gets SIGQUIT, so that its output shows the Go stacks of
// the process, and fails: a program that hung under qemu took the whole
// time of the test binary, which printed no stack of the program.
func hangCommand(t *testing.T, name string, args ...string) *exec.Cmd {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGQUIT) }
	cmd.WaitDelay = time.Minute
	return cmd
}

// TestPrintStreams checks the output streams of the print and println
// builtins, which TestTestdata cannot see (it compares the combined
// output): like gc, yaegi writes them to stderr, and fmt to stdout. With
// -print-stdout (yaegi's behavior, used by the conformance runner), yaegi v2
// writes everything to stdout, in the same order as the combined output.
func TestPrintStreams(t *testing.T) {
	for _, f := range []string{"printbuiltin.go", "printiface.go"} {
		t.Run(f, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(root, "testdata", f)
			run := func(name string, args ...string) (stdout, stderr []byte) {
				var so, se bytes.Buffer
				cmd := exec.Command(name, args...)
				cmd.Stdout, cmd.Stderr = &so, &se
				if err := cmd.Run(); err != nil {
					t.Fatalf("%s %v: %v\n%s%s", name, args, err, so.Bytes(), se.Bytes())
				}
				return maskAddrs(so.Bytes()), maskAddrs(se.Bytes())
			}
			wantOut, wantErr := run("go", "run", path)
			gotOut, gotErr := run(yaegiBin, path)
			if !bytes.Equal(gotOut, wantOut) {
				t.Errorf("stdout: got:\n%s\nwant (go run):\n%s", gotOut, wantOut)
			}
			if !bytes.Equal(gotErr, wantErr) {
				t.Errorf("stderr: got:\n%s\nwant (go run):\n%s", gotErr, wantErr)
			}
			if len(wantErr) == 0 {
				t.Errorf("go run wrote nothing to stderr: the program does not test print/println")
			}

			combined, err := exec.Command("go", "run", path).CombinedOutput()
			if err != nil {
				t.Fatalf("go run: %v\n%s", err, combined)
			}
			yOut, yErr := run(yaegiBin, "-print-stdout", path)
			if want := maskAddrs(combined); !bytes.Equal(yOut, want) {
				t.Errorf("-print-stdout: stdout: got:\n%s\nwant (go run, combined):\n%s", yOut, want)
			}
			if len(yErr) != 0 {
				t.Errorf("-print-stdout: stderr is not empty:\n%s", yErr)
			}
		})
	}
}

// TestTestdataRace runs the concurrent testdata programs with a yaegi v2 built
// with the race detector: the VM's own memory accesses (atomic intrinsics on
// interpreted cells) must be race-free, and the output must match go run.
// TestTraceback compares the traceback of an unrecovered panic
// (vm.PanicError.Trace, printed by yaegi) with the one of go run, for the
// programs of testdata/traceback. The programs must exit with status 2. The
// comparison keeps the frames of the program: the function name without
// its arguments (gc prints their values, yaegi v2 "(...)") and the file:line
// without the PC offset. The "created by" lines of gc (yaegi v2 has none) and
// the frames of the runtime are left out. The panic message is not
// compared: yaegi prints it in its own way.
func TestTraceback(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(root, "testdata", "traceback", "*.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no testdata/traceback programs: %v", err)
	}
	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()
			// go run exits with status 1 and prints the status of the
			// program.
			run := func(status int, name string, args ...string) string {
				out, err := exec.Command(name, args...).CombinedOutput()
				if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != status {
					t.Fatalf("%s: want exit status %d, got %v\n%s", name, status, err, out)
				}
				return programFrames(string(out), path)
			}
			want := run(1, "go", "run", path)
			got := run(2, yaegiBin, path)
			if want == "" {
				t.Fatalf("go run printed no frames of %s", path)
			}
			if got != want {
				t.Errorf("got:\n%s\nwant (go run):\n%s", got, want)
			}
		})
	}
}

// TestPanicLine checks the first line that yaegi prints for an unrecovered
// panic, in main and in a goroutine: "panic: x" as gc, not "panic: panic:
// x" (the Error of a *vm.VMPanic already starts with "panic: ").
func TestPanicLine(t *testing.T) {
	for _, name := range []string{"errorpanic.go", "goroutine.go", "nested.go"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(root, "testdata", "traceback", name)
			first := func(name string, args ...string) string {
				out, _ := exec.Command(name, args...).CombinedOutput()
				line, _, _ := strings.Cut(string(out), "\n")
				return line
			}
			want, got := first("go", "run", path), first(yaegiBin, path)
			if !strings.HasPrefix(want, "panic: ") || got != want {
				t.Errorf("first line: got %q, want (go run) %q", got, want)
			}
		})
	}
}

// programFrames returns the frames of file in the traceback out: one
// "function file:line" line per frame.
func programFrames(out, file string) string {
	var frames []string
	lines := strings.Split(out, "\n")
	start := false
	for i := 0; i+1 < len(lines); i++ {
		l := lines[i]
		if strings.HasPrefix(l, "goroutine ") && strings.HasSuffix(l, "[running]:") {
			start = true
			continue
		}
		if !start || strings.HasPrefix(l, "created by ") || !strings.HasPrefix(lines[i+1], "\t") {
			continue
		}
		loc := strings.Fields(lines[i+1])[0]
		if !strings.HasPrefix(loc, file+":") {
			continue
		}
		name := l
		if j := strings.LastIndexByte(name, '('); j > 0 {
			name = name[:j]
		}
		frames = append(frames, name+" "+filepath.Base(loc))
	}
	return strings.Join(frames, "\n")
}

func TestTestdataRace(t *testing.T) {
	if testing.Short() {
		t.Skip("builds yaegi v2 with -race")
	}
	race := filepath.Join(t.TempDir(), "yaegi-race")
	cmd := exec.Command("go", "build", "-race", "-o", race, "./cmd/yaegi")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("go build -race: %v\n%s", err, out)
	}
	for _, file := range []string{"atomics.go", "genericstd.go"} {
		path := filepath.Join(root, "testdata", file)
		want, err := exec.Command("go", "run", path).CombinedOutput()
		if err != nil {
			t.Fatalf("go run: %v\n%s", err, want)
		}
		got, err := exec.Command(race, path).CombinedOutput()
		if err != nil {
			t.Fatalf("yaegi -race: %v\n%s", err, got)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s: got:\n%s\nwant (go run):\n%s", file, got, want)
		}
	}
}

// TestRestrictedPrograms runs the sandbox programs of the yaegi study with
// the import restrictions of Traefik plugins: os/exec is refused at load
// time even when the call is never reached (s08) and even with -unsafe;
// unsafe needs -unsafe (s02).
func TestRestrictedPrograms(t *testing.T) {
	progs := filepath.Join(root, "compare", "yaegi", "progs")
	for _, c := range []struct {
		file  string
		flags []string
		rc    int
		out   string
	}{
		{"s01_os_exec.go", []string{"-restricted"}, 4, `import "os/exec" is not allowed`},
		{"s01_os_exec.go", []string{"-restricted", "-unsafe"}, 4, `import "os/exec" is not allowed`},
		{"s08_os_exec_unreached.go", []string{"-restricted"}, 4, `import "os/exec" is not allowed`},
		{"s02_unsafe_without_optin.go", []string{"-restricted"}, 4, `import "unsafe" is restricted`},
		{"s02_unsafe_without_optin.go", []string{"-restricted", "-unsafe"}, 0, "1 8\n"},
		{"s08_os_exec_unreached.go", nil, 0, "loaded\n"},
		// Without -restricted, os.Exit ends the program with its code and
		// skips the deferred calls, as in Go; with it, os.Exit panics.
		{"s04_os_exit.go", nil, 7, "before\n"},
		{"s04_os_exit.go", []string{"-restricted"}, 2, "before\ndeferred\npanic: os.Exit(7)"},
		{"s04_os_exit.go", []string{"-restrictedstd"}, 2, "before\ndeferred\npanic: os.Exit(7)"},
		// -stats prints the timings before the real os.Exit ends yaegi v2.
		{"s04_os_exit.go", []string{"-stats"}, 7, "before\nload+ssa: "},
		{"s01_os_exec.go", nil, 0, "pwned\n"},
	} {
		cmd := exec.Command(yaegiBin, append(append([]string{}, c.flags...), c.file)...)
		cmd.Dir = progs
		out, err := cmd.CombinedOutput()
		rc := 0
		if ee, ok := err.(*exec.ExitError); ok {
			rc = ee.ExitCode()
		} else if err != nil {
			t.Fatal(err)
		}
		if rc != c.rc || !strings.Contains(string(out), c.out) {
			t.Errorf("yaegi %v %s: exit %d, output %q; want exit %d with %q", c.flags, c.file, rc, out, c.rc, c.out)
		}
	}
}

// TestSyscallAddrRefused checks the two refusals around a uintptr address
// given to syscall.Syscall (testdata/syscalladdr.go): a plugin with
// useUnsafe cannot call syscall.Syscall at all, and without restrictions
// the address of a value that is not in real memory (a struct with a
// pointer field stays in cells of the VM) is refused before the call,
// instead of letting the kernel write over the cells.
func TestSyscallAddrRefused(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux system calls")
	}
	if why := only64["syscalladdr.go"]; strconv.IntSize < 64 {
		t.Skip(why)
	}
	cmd := exec.Command(yaegiBin, "-restricted", "-unsafe", filepath.Join(root, "testdata", "syscalladdr.go"))
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "native func syscall.Syscall not in registry") {
		t.Errorf("yaegi -restricted -unsafe syscalladdr.go: %v, output %q; want the refusal of syscall.Syscall", err, out)
	}

	src := filepath.Join(t.TempDir(), "main.go")
	prog := `package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

type withPtr struct {
	sec, nsec int64
	p         *int
}

func main() {
	var w withPtr
	fmt.Println("before")
	_, _, e := syscall.Syscall(syscall.SYS_CLOCK_GETTIME, 1, uintptr(unsafe.Pointer(&w)), 0)
	fmt.Println(e == 0, w.sec > 0)
}
`
	if err := os.WriteFile(src, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err = exec.Command(yaegiBin, src).CombinedOutput()
	want := "withPtr given to syscall.Syscall as a uintptr: the value is not in real memory"
	if err == nil || !strings.HasPrefix(string(out), "before\n") || !strings.Contains(string(out), want) || strings.Contains(string(out), "fatal error") {
		t.Errorf("yaegi (struct with a pointer): %v, output %q; want %q after \"before\"", err, out, want)
	}
}

// deepLevels returns s for this platform: on 32-bit platforms, the VM
// gives native code 125000 levels of interfaces, not 200000 (deepSegs in
// vm/native.go), so the value below the bound has 120000 levels, not
// 150000.
func deepLevels(s string) string {
	if strconv.IntSize == 64 {
		return s
	}
	return strings.NewReplacer("150_000", "120_000", "149_995:150_005", "119_995:120_005",
		"300001", "240001", "150000", "120000").Replace(s)
}

// TestCLIDeepPrint prints values nested in interfaces very deep. fmt
// recurses on the host stack: a value nested about 300000 times killed
// yaegi v2 (and a Traefik host) with a fatal stack overflow, in every mode.
// Up to 200000 levels the output is the one of Go (125000 on 32-bit
// platforms, whose goroutine stacks are 4 times smaller); deeper, fmt shows
// the panic of the VM in the output and the program goes on.
func TestCLIDeepPrint(t *testing.T) {
	src := filepath.Join(t.TempDir(), "main.go")
	prog := deepLevels(`package main

import (
	"fmt"
	"strings"
)

func nest(n int) any {
	var v any = 0
	for i := 0; i < n; i++ {
		v = []any{v}
	}
	return v
}

type box struct{ V any }

func main() {
	s := fmt.Sprint(nest(150_000))
	fmt.Println(len(s), strings.Count(s, "["), s[149_995:150_005])
	for _, x := range []any{nest(1_000_000), map[string]any{"k": nest(400_000)}, box{nest(400_000)}, &box{nest(400_000)}} {
		s := fmt.Sprintf("%v", x)
		fmt.Println(strings.Contains(s, "times in interfaces is too deep"), len(s) < 1_000_000)
	}
	fmt.Println("end")
}
`)
	if err := os.WriteFile(src, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	want := deepLevels("300001 150000 [[[[[0]]]]\n") + "true true\ntrue true\ntrue true\ntrue true\nend\n"
	for _, flags := range [][]string{nil, {"-aot"}, {"-restricted", "-aot"}} {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		out, err := exec.CommandContext(ctx, yaegiBin, append(append([]string{}, flags...), src)...).CombinedOutput()
		cancel()
		if err != nil || string(out) != want {
			t.Errorf("yaegi %v: %v, output %.300q; want %q", flags, err, out, want)
		}
	}
}

// TestCLIDeepJSON encodes values nested in interfaces very deep with
// encoding/json. The AOT copy of json recursed through the twin to a fatal
// stack overflow near 1M levels, and native json near 400000 levels (the
// part past maxBoxDepth runs on top of the encoder): both killed yaegi v2.
// Past 200000 levels, Marshal, MarshalIndent and Encoder.Encode now return
// an error; below, the AOT copy gives the output of Go (also in default
// mode, where json.Marshal goes to it): json v2 (Go 1.27) encodes 10000
// levels and returns "exceeded max depth" past them, json v1 (Go 1.26)
// encodes the 150000 levels.
func TestCLIDeepJSON(t *testing.T) {
	src := filepath.Join(t.TempDir(), "main.go")
	prog := deepLevels(`package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func nest(n int) any {
	var v any = 0
	for i := 0; i < n; i++ {
		v = []any{v}
	}
	return v
}

func main() {
	if len(os.Args) > 1 {
		for _, n := range []int{10_000, 150_000} {
			b, err := json.Marshal(nest(n))
			fmt.Println(len(b), err)
		}
	}
	for _, n := range []int{400_000, 1_000_000} {
		v := nest(n)
		_, err1 := json.Marshal(v)
		_, err2 := json.MarshalIndent(map[string]any{"k": v}, "", " ")
		err3 := json.NewEncoder(&bytes.Buffer{}).Encode([]any{v})
		for _, err := range []error{err1, err2, err3} {
			fmt.Print(err != nil && strings.Contains(err.Error(), "times in interfaces is too deep"), " ")
		}
		fmt.Println()
	}
	fmt.Println("end")
}
`)
	if err := os.WriteFile(src, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	const tooDeep = "true true true \ntrue true true \nend\n"
	ok := "20001 <nil>\n0 exceeded max depth\n"
	if !haveJSONv2 {
		ok = "20001 <nil>\n300001 <nil>\n"
	}
	for _, c := range []struct {
		flags []string
		want  string
	}{
		// default mode: json.Marshal goes to the AOT copy (aotroute.go)
		{nil, ok + tooDeep},
		{[]string{"-aot"}, ok + tooDeep},
		{[]string{"-restricted", "-aot"}, ok + tooDeep},
	} {
		args := append(append([]string{}, c.flags...), src, "150k")
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		out, err := exec.CommandContext(ctx, yaegiBin, args...).CombinedOutput()
		cancel()
		if err != nil || string(out) != c.want {
			t.Errorf("yaegi %v: %v, output %.300q; want %q", c.flags, err, out, c.want)
		}
	}
}

// TestCLIDeepWalkers walks a value nested 1M times in interfaces with AOT
// copies other than fmt and json, which have no bound of their own. They
// recurse once per level through the twin reflect: yaml.v3, toml, go-cmp,
// asn1, text/template, and cast (through the AOT copy of fmt, called by
// another AOT copy) reached the stack limit of the goroutine, a fatal error
// that killed yaegi v2 (and a Traefik host). RV.Elem now panics before the
// limit (vm/stackguard.go): the program can recover, and it goes on.
func TestCLIDeepWalkers(t *testing.T) {
	src := filepath.Join(t.TempDir(), "main.go")
	const prog = `package main

import (
	"bytes"
	"encoding/asn1"
	"fmt"
	"strings"
	"text/template"

	"github.com/BurntSushi/toml"
	"github.com/google/go-cmp/cmp"
	"github.com/spf13/cast"
	"gopkg.in/yaml.v3"
)

func nest(n int) any {
	var v any = 0
	for i := 0; i < n; i++ {
		v = []any{v}
	}
	return v
}

func try(name string, f func() error) {
	defer func() {
		r := recover()
		err, _ := r.(error)
		fmt.Println(name, err != nil && strings.Contains(err.Error(), "nested too deep"))
	}()
	f()
}

func main() {
	v := nest(1_000_000)
	try("yaml", func() error { _, err := yaml.Marshal(v); return err })
	// toml uses less stack per level with Go 1.27: it encodes 1M levels (as
	// go run does) and reaches the limit near 2M
	try("toml", func() error { return toml.NewEncoder(&bytes.Buffer{}).Encode(map[string]any{"k": nest(2_000_000)}) })
	try("cmp", func() error { cmp.Equal(v, v); return nil })
	try("cast", func() error { _, err := cast.ToStringE(v); return err })
	try("asn1", func() error { _, err := asn1.Marshal(v); return err })
	try("template", func() error {
		return template.Must(template.New("").Parse("{{.}}")).Execute(&bytes.Buffer{}, v)
	})
	fmt.Println("end")
}
`
	if err := os.WriteFile(src, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	const want = "yaml true\ntoml true\ncmp true\ncast true\nasn1 true\ntemplate true\nend\n"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, yaegiBin, "-aot", src).CombinedOutput()
	if err != nil || string(out) != want {
		t.Errorf("yaegi -aot: %v, output %.600q; want %q", err, out, want)
	}
}

// TestCLIGoexit checks runtime.Goexit in the main goroutine: as with gc,
// the other goroutines go on, then yaegi v2 stops with the gc fatal error and
// exit code 2 (it must not hang).
func TestCLIGoexit(t *testing.T) {
	const fatal = "fatal error: no goroutines (main called runtime.Goexit) - deadlock!\n"
	for _, c := range []struct{ name, body, want string }{
		{"direct", `ch := make(chan bool)
	go func() {
		<-ch
		fmt.Println("other goroutine")
	}()
	defer close(ch)
	defer fmt.Println("deferred")
	runtime.Goexit()`, "deferred\nother goroutine\n" + fatal},
		// Native code (sync.Once.Do) calls the func value on the main
		// goroutine, which runs VM code.
		{"once.Do", `defer fmt.Println("deferred")
	var o sync.Once
	o.Do(runtime.Goexit)`, "deferred\n" + fatal},
	} {
		src := filepath.Join(t.TempDir(), "main.go")
		prog := "package main\n\nimport (\n\t\"fmt\"\n\t\"runtime\"\n\t\"sync\"\n)\n\nvar _ sync.Once\n\nfunc main() {\n\t" + c.body + "\n}\n"
		if err := os.WriteFile(src, []byte(prog), 0o644); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		out, err := exec.CommandContext(ctx, yaegiBin, src).CombinedOutput()
		cancel()
		rc := 0
		if ee, ok := err.(*exec.ExitError); ok {
			rc = ee.ExitCode()
		} else if err != nil {
			t.Fatal(err)
		}
		if rc != 2 || string(out) != c.want {
			t.Errorf("%s: yaegi: exit %d, output %q; want exit 2 with %q", c.name, rc, out, c.want)
		}
	}
}

// TestPrintLargeValue checks the cost of a large value given to native
// printers in plugin mode (-restricted -aot). Go does not read the elements
// for %T, and a logger that writes to io.Discard formats nothing. With
// OpaqueAny the value was an opaque handle (O(1)); since the printers get a
// native copy, each call copied the 2 million elements, and the budget did
// not count it: 800 calls took more than a minute (0.1 s on main). The copy
// for printers is now bounded (printCopyMax), and a larger value gets the
// handle again.
func TestPrintLargeValue(t *testing.T) {
	src := filepath.Join(t.TempDir(), "main.go")
	prog := `package main

import (
	"fmt"
	"io"
	"log"
)

func main() {
	big := make([]int, 2_000_000)
	m := map[int][]int{1: big}
	l := log.New(io.Discard, "", 0)
	for i := 0; i < 400; i++ {
		l.Printf("%T %v", big, m)
		_ = fmt.Errorf("%T", []any{big})
	}
	fmt.Println(fmt.Sprintf("%T %v %T", []int{1, 2}, []int{1, 2}, map[string]int{"a": 1}))
}
`
	if err := os.WriteFile(src, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, yaegiBin, "-restricted", "-aot", "-budget", "1000000", src).CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("yaegi did not end in 30s: the copy of a large value for a printer is not bounded")
	}
	const want = "[]int [1 2] map[string]int\n"
	if err != nil || string(out) != want {
		t.Errorf("yaegi: %v, output %q; want %q", err, out, want)
	}
}

// baseline reads docs/conformance/results.tsv: file -> group.
func baseline(t *testing.T) (files []string, group map[string]string) {
	t.Helper()
	f, err := os.Open(filepath.Join(root, "docs", "conformance", "results.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	group = map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		cols := strings.Split(sc.Text(), "\t")
		if len(cols) < 5 || cols[0] == "file" {
			continue
		}
		files = append(files, cols[0])
		group[cols[0]] = cols[4]
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return files, group
}

func checkConformance(t *testing.T, files []string, group map[string]string) {
	t.Helper()
	paths := make([]string, len(files))
	for i, f := range files {
		paths[i] = filepath.Join(wsDir, f)
	}
	cfg := Config{VM: yaegiBin, Jobs: runtime.NumCPU(), Timeout: conformanceTimeout(t)}
	results := Run(cfg, wsDir, paths, ReadOrigins(filepath.Join(root, "tests", "yaegi", "MANIFEST.tsv")))
	var improved []string
	for _, r := range results {
		was, now := group[r.File], Group(r.Status)
		switch {
		case was == "pass" && now != "pass":
			t.Errorf("%s: regression: %s %s", r.File, r.Status, r.Detail)
		case was != "pass" && now == "pass":
			improved = append(improved, r.File)
		}
	}
	if len(improved) > 0 {
		t.Logf("now passing (update docs/conformance with go run ./cmd/runtests): %s", strings.Join(improved, " "))
	}
}

// conformanceTimeout is the time limit of one conformance program: 30s, or
// $YAEGI_CONFORMANCE_TIMEOUT (a time.Duration). Under qemu-user (the linux/arm
// and linux/s390x jobs of CI), loading net/http alone takes longer than 30s.
func conformanceTimeout(t *testing.T) time.Duration {
	s := os.Getenv("YAEGI_CONFORMANCE_TIMEOUT")
	if s == "" {
		return 30 * time.Second
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		t.Fatalf("YAEGI_CONFORMANCE_TIMEOUT: %v", err)
	}
	return d
}

// TestConformanceSample runs a deterministic sample (one in six) of the
// conformance programs that pass in the baseline.
func TestConformanceSample(t *testing.T) {
	files, group := baseline(t)
	var sample []string
	n := 0
	for _, f := range files {
		if group[f] != "pass" {
			continue
		}
		if n%6 == 0 {
			sample = append(sample, f)
		}
		n++
	}
	checkConformance(t, sample, group)
}

// TestConformanceFull runs every conformance program and reports regressions
// against the baseline. Opt-in: YAEGI_CONFORMANCE=1 go test ./cmd/runtests -run Full.
func TestConformanceFull(t *testing.T) {
	if os.Getenv("YAEGI_CONFORMANCE") == "" {
		t.Skip("set YAEGI_CONFORMANCE=1 to run the whole conformance suite")
	}
	files, err := Discover(wsDir)
	if err != nil {
		t.Fatal(err)
	}
	_, group := baseline(t)
	names := make([]string, len(files))
	for i, f := range files {
		names[i] = filepath.Base(f)
	}
	checkConformance(t, names, group)
}
