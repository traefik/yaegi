# `unsafe` in the VM

The VM has no "byte" memory. A variable is a cell `Value{N int64; R any}`.
A struct or an array is a `[]Value` (one cell per field or element). A string is a Go `string`.
A `[]byte` is a Go `[]byte` when it comes from `make`, from `[]byte(s)` or from native code,
and a `[]Value` when it is a literal (`[]byte{1, 2}`) or a slice of a `[N]byte` array that is also used as an array. An `unsafe.Pointer`
is the interpreted pointer itself (`*Value`, or `*byte` into a `[]byte` or into the bytes of a string).
So `unsafe` is emulated on this model, not executed. Everything that depends only on object identity,
sizes and offsets works, and so does everything that only looks at the data of a string or a `[]byte`:
these are real Go memory. Everything that assumes reading bytes again in another
form cannot work. Code: `vm/unsafe.go`, `vm/unsafehdr.go`, `vm/ifaceview.go`. Non-regression
tests: `testdata/unsafe.go`, `testdata/unsafehdr.go`, `testdata/unsafeview.go`, `testdata/unsafeiface.go`,
`testdata/unsafemsg.go`, `TestUnsafeHeaders` (refusals).

**protobuf** (google.golang.org/protobuf v1.36): a plugin with its own generated messages runs as with gc
(`TestProtobufPlugin`, `vm/testdata/pbplug`: oneof, proto3 optional, maps, repeated fields, enums, `Any`,
`Timestamp`, `Struct`; Marshal, Unmarshal, Equal, Clone, Merge, Reset, protojson, prototext, protoreflect; on
8 goroutines). The plugin needs `useUnsafe`. A round trip of a medium message is about 200 times slower than
gc (`BenchmarkProtobuf`).

**json-iterator** (with modern-go/reflect2) runs as with gc (`testdata/jsoniter.go`): maps, slices, structs,
pointers and interfaces, encode and decode. reflect2 calls functions of the runtime with `//go:linkname`;
`vm/reflect2.go` emulates them on interpreted values (map iteration, `unsafe_New`, `unsafe_NewArray`,
`typedmemmove`, `typedslicecopy`, `makemap`, `mapassign`, `mapaccess`, `ifaceE2I`). The key and value
pointers of a map iteration point to copies: a write through them does not reach the map. A native
`reflect.Type` (a type of the host) is refused. A cell of a slice looks like a cell of a struct, so
`(*sliceHeader)(p)` needs to know the type: the data word of a slice in an interface is a view of its header,
a `*[]T` gets a hint, and a slice that is the first field of a struct is found through the struct.

**Functions linked to the runtime** (`vm/linkname.go`, `testdata/linkruntime.go`): `runtime.memmove` copies `n`
bytes between two places of bytes that the VM knows (a Go `[]byte`, the bytes of a string, plain data in real bytes),
with the checks of `unsafe.Slice` below: a copy past these bytes, or into the cells of other values, is refused
(`TestMemmoveChecks`). When the SSA shows the same type `T` behind both pointers (`memmove(unsafe.Pointer(&b), unsafe.Pointer(&a), unsafe.Sizeof(a))`)
and `n` is the size of `T`, the call is the assignment `b = a`, also for a value held in cells (a struct with a string, a native
`time.Time`: `testdata/memmovetyped.go`); a part of such a value stays refused. `runtime.nanotime` and `runtime.getAuxv` (a copy) are given as in Go. As in gc, the directive
counts only in a file that imports `unsafe`, and the function must have the signature of the runtime function
(`TestLinknameRules`). Other functions without a Go body are refused at load time, or are stubs with
`Options.StubAssembly` (`docs/SANDBOX.md`).

**Random and hash functions of the runtime** (`vm/linkrand.go`, `testdata/linkrand.go`): the functions that many modules
link to (`cheaprand`, `cheaprandn`, `cheaprand64`, `fastrand`, `fastrandn`, `fastrand64`, `rand`, `randn`, `memhash`,
`memhash32`, `memhash64`, `strhash`) are emulated with the standard library: random numbers of the same range
(math/rand/v2), and the same hash for the same bytes and seed in one process (hash/maphash), not the values of the runtime,
which change at each start in gc too. A hash of a value held in cells is refused (`TestLinkRandChecks`). This table follows
the runtime of a Go release: a new name stays refused until it is added.

`//go:embed` into a `string`, a `[]byte` or an `embed.FS` of a package run from source works (`vm/embed.go`,
`testdata/goembed.go`, `testdata/goembedfs.go`): protobuf embeds its edition defaults, OPA its capability files.
The files are selected with the rules of the go command (`all:`, names that start with `.` or `_`, no symbolic link,
no other module, nothing outside the package directory: `TestEmbedFSRules`), and the total size and number of files
are bounded with the other load limits (`LoadLimits`). An `embed.FS` is the real type of package `embed`, filled
with its files in the order of gc: its methods, `io/fs` and `net/http` see it as in Go.

## What works

| Use | Behavior |
|---|---|
| `unsafe.Sizeof`, `Alignof`, `Offsetof` | constants computed by go/types with the gc sizes (`types.SizesFor("gc", GOARCH)`), identical to `go run` |
| `unsafe.Sizeof`, `Alignof`, `Offsetof` on an operand that depends on a type parameter | not constants for go/types: ssa keeps a builtin call in each generic instance. The VM computes the value when it compiles the instance, with the same gc sizes, and keeps it in a constant register. The operand is not evaluated (as in Go): its instructions are not compiled when they are used only by the call (`unsafe.Sizeof(*p)` with a nil `p` does not panic, `unsafe.Sizeof(f())` does not call `f`). This includes composite literals: their Alloc, element addresses, Store and MapUpdate instructions are not compiled either (`unsafe.Sizeof(S[K]{B: *p})` does not panic). Limits: this needs the syntax of the function (always there for source packages). For `Offsetof(x.f)`, the steps through embedded fields are found from their ssa position; without syntax, an `Offsetof` with more than one step is `unsupported` (never a wrong offset). An operand instruction that is also used elsewhere is still run |
| `unsafe.StringData`, `SliceData` | `StringData` gives a `*byte` to the bytes of the string (read-only, as in gc: a write into a string outside the Go heap, a constant in read-only memory, is refused instead of stopping the process; a `[]byte` made from such bytes by `unsafe.Slice` or a header view is a copy, `testdata/unsafero.go`); `SliceData` the pointer to the first element (a `*byte` for a Go `[]byte`, a `*Value` for a slice of cells) |
| `unsafe.String(p, n)`, `unsafe.Slice(p, n)` | the result shares the memory of `p`, as in gc: `unsafe.String(&b[0], len(b))` sees later writes into `b` when `b` is a Go `[]byte`; for bytes in cells (a `[]byte` literal, a `[N]byte` array) the string is a copy. `unsafe.Slice` works for every element type (cells or bytes); over the real bytes of a Go `[]byte`, a slice of an unnamed integer or float type wider than a byte (`unsafe.Slice((*uint32)(unsafe.Pointer(&b[0])), len(b)/4)`) is a Go slice over the same bytes (a native slice in the VM). The panics of gc are the same: nil pointer with a non-zero length, negative or too large length. The VM also checks that the `n` elements are inside the array behind `p` (see "Checks" below) |
| String and slice headers: `(*reflect.StringHeader)(unsafe.Pointer(&s))`, `(*reflect.SliceHeader)(unsafe.Pointer(&b))`, a local struct of words (`struct{ p unsafe.Pointer; n int }`, `struct{ data uintptr; len, cap int }`), `[3]int`, `*(*uintptr)(unsafe.Pointer(&s))`, `*(*int)(unsafe.Pointer(uintptr(unsafe.Pointer(&s)) + 8))` | the pointer is a view of the variable (`hdrView`). Reading a word gives the data address, the length or the capacity of the current value. Writing a word builds a new value by re-slicing the real backing array of the data, and puts it in the variable: `h.Len = 2`, `h.Data += 7`, the old `b2s`/`s2b` idioms (`sh.Data = bh.Data; sh.Len = bh.Len`) and a slice header filled field by field (`Len`, then `Cap`, then `Data`) work. A header that is not valid yet (length above capacity, data not written yet, or data moved first so that the old length and capacity reach past the array: `h.Data += 2` before `h.Len -= 2; h.Cap -= 2`) is kept by the view until it becomes valid; a length or a capacity written outside the array is refused at once. `unsafe.Pointer(h.Data)` gives back the pointer |
| A struct of a string and a word read as a slice: `*(*[]byte)(unsafe.Pointer(&sliceHeader{s, len(s)}))` with `type sliceHeader struct{ s string; cap int }` (cespare/xxhash `Sum64String`, `WriteString`) | the slice shares the bytes of the string (a copy for a string outside the heap); the capacity must stay inside the string |
| `*(*string)(unsafe.Pointer(&b))` and `*(*[]byte)(unsafe.Pointer(&s))` | zero copy, as in gc, when the bytes are Go memory: the string sees later writes into `b`. The `[]byte` made from a string shares its bytes (capacity = length), except for a string outside the Go heap (a copy). A header struct built by the program and read as a string or a slice (`*(*string)(unsafe.Pointer(&sh))`) works too, but the result is a new variable |
| `(*[N]T)(unsafe.Pointer(&s[i]))` | the array is made of the `N` cells from `s[i]`: they are shared with the slice, as in gc. `N` must fit in the backing array. `(*T)(unsafe.Pointer(&arr))` gives the first element |
| `(*T)(unsafe.Pointer(p))` with `p` of type `*F` | allowed when `F` and `T` have the same representation in the VM and in gc: integers and booleans of the same size, `float64`↔`uint64` (the cell keeps the IEEE bits), pointers between them, structs and arrays with compatible fields. The two views share the cell |
| A value of a native type of plain data seen as another type that fits inside it: `(*syscall.RawSockaddrInet4)(unsafe.Pointer(&rsa))` for a `syscall.RawSockaddrAny` | a native value lives in native memory with gc's layout, so the view is the same memory: a native `T` is `reflect.NewAt(T, address)`, an interpreted `T` of plain data is a view of the bytes (`vm/nativeview.go`, `testdata/nativeprefixview.go`). A native `T` of plain data over real bytes (`(*unix.RawSockaddrInet4)(unsafe.Pointer(&b[i]))` over a Go `[]byte` or a local `[16]byte` viewed as another type, as x/net and x/sys parse socket messages) is `reflect.NewAt` at the address of the bytes, inside their array. Both types must be plain data (no pointer, string, slice, map, chan, func or interface word), `T` must not be larger than the value or reach past the bytes, and the address must be aligned for `T`; bytes of a string constant (read-only) and, under the restrictions of plugins, memory of the host are refused too (`TestNativeViewChecks`, `TestXSysHostPointers`). Before, `T` fitting or not, the conversion was refused, and with a pointer whose origin the SSA does not show (an `unsafe.Pointer` parameter) the view kept the native type and a store through it panicked in reflect |
| `uintptr(p)` | the address of the array that holds `p` plus the gc offset of the element, when the VM knows that array (the SSA shows `&a[i]`, or a recent region holds `p`): `uintptr(unsafe.Pointer(&a[1])) - uintptr(unsafe.Pointer(&a[0]))` is `Sizeof(a[0])`, as in gc (`fixedbugs/issue36516.go`, `testdata/uintptrelem.go`); `uintptr(unsafe.Pointer(&a))` of an array is the address of `&a[0]`, as in gc, and goes back to the array (`slice3.go`, `testdata/uintptrarray.go`); else the real address of the cell (or of the byte). Printing (`%x`, `%p`, `println`), comparisons, map keys, and going back with `unsafe.Pointer(uintptr(p))` give the expected results. `uintptr(unsafe.Pointer(&b[0]))` is the same as the `Data` of the header and `unsafe.SliceData(b)` |
| `unsafe.Add(p, off)`, `unsafe.Pointer(uintptr(p) + off)` | the offset, computed with the gc sizes, is resolved to the cell of the field or element at that place: a struct field (`Offsetof`), an array element (forward and backward from `&a[i]`), a slice element when `p` comes from `&s[i]` (you can then move up to the capacity of `s`, and back to `s[0]`), recursively in nested structs/arrays |
| `unsafe.Pointer(u)` for an address `u` that is not in known memory (a constant, the middle of an `int64`, past the end of an object) | as in gc, the pointer exists: it can be stored, compared, converted back to `uintptr` and moved back into an object (`uintptr(b) + 2 - 1 &^ 1`). Only a dereference is refused (`unsupported`), and so are `unsafe.Slice` of it and passing it to native code |
| `unsafe.Pointer` to native code | passed as the Go pointer of the cell (`%p`, `%x` of native `fmt`); a `*byte` into a Go `[]byte` is the real pointer, shared with native code |
| The two words of an interface: `(*ifaceHeader)(unsafe.Pointer(&v))` with a struct of two words (protobuf's `protoreflect.ifaceHeader`, `impl.pointerOfIface`, reflect2's `eface`), `(*[2]uintptr)(unsafe.Pointer(&v))[1] == 0` (zerolog's `isNilValue`) | a view of the interface variable (`vm/ifaceview.go`). The type word is a token: a cell unique to the dynamic type, so two values of the same type have equal type words, as in gc. The data word is the pointer itself for a pointer-shaped type (pointer, map, chan, func, `unsafe.Pointer`), else a pointer to a copy of the value; read as a `uintptr` it is 0 for a nil pointer. The data word of a `reflect.Type` is the type word of the values of that type, as in gc (reflect2 caches its types by both). Writing both words builds an interface again; the type word must come from an interface (a token), never from an integer, and the type must implement the static interface type. `testdata/unsafeiface.go` |
| The `flag` field of a `reflect.Value`, read and written at the offset of `reflect.TypeOf(reflect.Value{}).FieldByName("flag")` (go-spew's `bypass.go`, pulled in by client-go) | a `reflect.Value` of the VM has the three fields of reflect's (`typ_`, `ptr`, `flag`), and its flag word uses the bits of gc for read-only (`1<<5`, `1<<6`) and addressable (`1<<8`); the kind bits and `flagIndir` are not there. go-spew's init check passes, and clearing the read-only bits makes `Interface` work on an unexported field of a plugin value, as in gc. For a value of a host type (native data), the read-only state of native reflect stays: `Interface` of its unexported fields still panics, and it is never settable; a byte of a string in read-only memory is a copy, so clearing its bit writes nothing (`testdata/reflectflag.go`, `TestReflectFlagForge`) |
| A struct and its first field at the same address: `(*messageState)(unsafe.Pointer(m))` for a `*Msg` whose first field is a `messageState`, and back with `reflect.NewAt(reflect.TypeOf(Msg{}), unsafe.Pointer(ms))` (protobuf's `MessageStateOf`, `messageState.pointer()`, the generated `Reset`) | the first field (n levels down) is the field cell, and the struct is recorded at the address of that cell, so that `uintptr(unsafe.Pointer(ms)) + off` reaches the other fields of the message, and a view of that address as the struct gives the struct cell again. `testdata/unsafemsg.go` |
| Fields reached with the offsets of `reflect.StructField` from a pointer of unknown type (protobuf's `pointer.Apply`, `Elem`, `PointerSlice`, `AppendPointerSlice`, `AsValueOf`) | `unsafe.Pointer(p)` of a `*S` (and `reflect.Value.Pointer()` of one) records the struct `S`, and a field pointer found by an offset records the struct its pointer field points to. A slice field seen as `*unsafe.Pointer` gives its data pointer, a map field the map (nil or not, as in gc). A `[]*T` seen as a `[]struct{ p unsafe.Pointer }` is a **copy** of the elements in the other form, both ways: a write to an element of one slice is not seen in the other; appending and storing the whole slice back (protobuf's only use) works. The type of a struct reached this way is also kept by a weak table (`ptrHints`), so that it is found again when the 64 regions have changed (protobuf's `proto.Equal` walks a tree of messages and comes back to the outer one). `reflect.NewAt(t, p)` takes the cell of the struct of type `t` at that address |
| Plain data over real bytes: `(*T)(unsafe.Pointer(&b[i]))`, `(*T)(unsafe.Add(p, n))`, `(*[N]byte)(unsafe.Pointer(&b[i]))` when `b` is a Go `[]byte` (from `make`, a conversion, a `[]byte{...}` literal, native code) or the bytes of a string, and `T` has no pointer, string, slice, map, chan, func or interface (integers, floats, booleans, structs and arrays of them) | a view of the bytes (`vm/byteview.go`): loads decode and stores encode in the host order, with gc's field offsets; `&p.f`, `&p[i]`, `unsafe.Slice((*byte)(p), n)` and `p[:]` stay in the same bytes; `==` compares addresses; native code gets the real pointer (`reflect.NewAt`). Every access is a bounds-checked slice access, so a view never reaches outside the array; a view of a string outside the heap is read-only. The word read of xxhash and the entry header of freecache work this way. Converting a view to another pointer type goes through the same check, whether the SSA shows the source type or not (`(*S)(p)` with `p` an `unsafe.Pointer` parameter): a `S` with a pointer, a string or an interface word is refused (`needs plain data`), so native code never reads a pointer word out of bytes the program wrote. A `(*[N]byte)` over a `[N]byte` array held as cells still shares the cells, and a one-byte view (`*int8`, `*bool`) over a byte gives the canonical value (an `int8` is sign extended) |

### Checks

The VM never makes a Go pointer from an integer, and a string or a slice that it makes for `unsafe`
is always a part of a real Go array that it holds. It knows the extent of the memory behind a pointer from:

- the SSA of the function: `&x[i]` (the backing array of the slice, or the array), `unsafe.SliceData(x)`, `unsafe.StringData(s)`;
- the last 64 regions converted to `uintptr`, made into `unsafe.Pointer` from `&x[i]`, or read as the data of a header, per goroutine (the other goroutines cannot push them out: `testdata/unsafegoroutines.go`), then the last 64 of the whole machine (a `uintptr` made on another goroutine). They are held by weak pointers: as in gc, a `uintptr` does not keep its object alive (its finalizer can run, `fixedbugs/issue45045.go`);
- the type of a struct reached through unsafe (`ptrHints`, held by weak pointers), when the regions forgot it;
- for `unsafe.Slice` and `unsafe.String`, the extent of each pointer that `unsafe.SliceData` or `unsafe.StringData`
  gave (`vm/unsafeext.go`, held by weak pointers, memory in the Go heap only), when the regions forgot it: protobuf
  keeps the data pointer and the length of a `[]byte` in its `protoreflect.Value` and makes the slice again much later
  (`testdata/unsafeextent.go`, the init of cel-go);
- otherwise, the single element the pointer points to.

The addresses the program sees for the fields of a struct (its address plus a gc offset) are not real
addresses: they can fall inside the cell of another object. A lookup takes the newest region that holds the
address, so `uintptr(p)` records the region of `p` again unless it is the newest one (`uregions.knows`):
otherwise, under load, `uintptr(p) + off` found a field of another struct (protobuf's sizecache read the
header of a slice, `testdata/protobuf.go`).

A length, a capacity or a data address outside this memory is refused (`unsupported: unsafe: ... reaches outside the memory known to the VM`).
The refusal is a panic: the program can recover it. This is the protection of plugins with `useUnsafe`
(`docs/SANDBOX.md`). It also refuses some correct programs: a pointer that comes from another function, with a long
`unsafe.Slice`, after the 64 regions have changed, when the pointer was not given by `SliceData` or `StringData`
(an `&b[i]`), or points to a string constant of the host.

## What cannot work in this value model

- **Reinterpreting cells**: a slice of cells seen as another element type (`[]uint32{...}` as a `[]byte`), or a variable
  whose address reaches the conversion by a path the SSA does not show (a parameter, a field of another variable). The
  bytes do not exist: a `[16]byte` array held as cells is 16 cells and a `uint16` is a single one. The VM reports these
  conversions as `unsupported: unsafe: viewing a F as a T needs byte-level memory` instead of producing wrong values
  (yaegi program `unsafe3`). A variable of plain data whose address the SSA converts to another plain type, directly or
  through a variable that holds it (also captured by a closure), lives in real bytes instead (`vm/byteview.go`,
  `bytesAlloc`): a local `[8]byte` read as a `uint64`, a struct seen as `[N]byte` or laid over with another struct
  (`fixedbugs/issue4585.go`, the entry header of freecache over a local `[24]byte`), a `uint16` as a `[2]uint8`, a
  `float32` as a `uint32` (`testdata/unsafebytes.go`). So does a variable of plain data whose field or element address
  goes to native code (put in an interface, or passed to a native function, also by `go` or `defer`, `innerToNative`),
  unless the program has its own reflect (it imports `reflect`, `ownReflect`) and the address of the whole variable
  also escapes (`&c` in an interface, in a call, in a closure: `addrEscapes`; it then keeps its cells, which native
  code copies and writes back, so the reflect of the program still sets it through `&c`): native code then sees
  gc's layout (`%p` of `&a[0]` and `&a[1]` differ by the size of the element, `fixedbugs/bug260.go`) and writes into
  the variable itself, also later (`flag.IntVar(&c.N, ...)`, `testdata/plainaddrs.go`). Its loads and stores decode and encode the bytes, native code gets
  the real pointer, and sync/atomic works on its aligned integers. A slice of such an array (`a[1:3]` of a
  `[4]uint16`) is a Go slice over the same bytes. The reflect of the program reads it through a copy: `Elem` of a
  pointer to it is not addressable, so a `reflect.Value.Set*` on it panics (`using unaddressable value`), where gc
  sets it. Bytes that are real (a Go `[]byte`, a string) can be
  viewed as plain data too (see above).
- **The same element seen from two subslices**: the `uintptr` of an element depends on the subslice it comes from.
  `&a[2]` taken from `a[1:]` and from `a[2:]` gives two different values, where gc gives one. A region starts at
  the cell of its first element, and the cells are larger than the elements of gc. Code that
  compares such addresses to know whether two slices overlap is wrong. `slices.Insert` and `slices.Replace` do it
  in `slices.overlaps`: the VM implements that function with the real memory of the two slices
  (`vm/slicesoverlap.go`).
- **Addresses given to system calls**: `syscall.Syscall(SYS_CLOCK_GETTIME, 1, uintptr(unsafe.Pointer(&ts)), 0)`
  (rule 4 of `unsafe.Pointer`) gives the kernel an address where it reads and writes with gc's layout. A variable of
  plain data whose address (or the address of a field or an element) is converted to a `uintptr` in the arguments of
  a native function lives in real bytes (`uintptrToNative`, `bytesAlloc`), and a variable of a native struct type
  (`syscall.Timespec`) is already native memory: its address is the address of that memory, the same as the
  address of its first field. The kernel then writes the variable itself (`testdata/syscalladdr.go`). The address of
  a value that is not in real memory (a struct with a pointer field, an element of a slice of cells) converted to a
  `uintptr` for a function of package `syscall` is refused before the call
  (`unsupported: unsafe: the address of a *T given to F as a uintptr: the value is not in real memory`): before,
  the kernel wrote over the cells and the VM stopped with a fault. Other native functions take a `uintptr` as a
  number (`atomic.StoreUintptr(&p, uintptr(unsafe.Pointer(&v)))`): a cell address is not refused there, and
  `unsafe.Pointer(atomic.LoadUintptr(&p))` gives the pointer back (`testdata/atomicuintptr.go`). A plugin with
  `useUnsafe` cannot call `syscall.Syscall` at all (docs/SANDBOX.md).
- **The layout of runtime types and functions**: the type word of an interface is a token, not a `runtime._type`:
  reading the fields of the type behind it (`(*eface)(unsafe.Pointer(&v)).typ.kind`, `fixedbugs/issue52612.go`) is
  refused. A function is a `*Closure`, not a funcval: only its first word, the code pointer, is given. The data word
  of a func in an interface, read as a pointer to an integer word (`**(**uintptr)(unsafe.Pointer(uintptr(unsafe.Pointer(&f)) + 8))`,
  the funcPC of `fixedbugs/issue17381.go`), points to a copy of that word: the address that
  `reflect.ValueOf(f).Pointer()` gives, which `runtime.FuncForPC` names (`testdata/funccodeword.go`). The captured
  variables of a closure are not behind it.
- **Addresses from native code**: native code sees a copy of interpreted memory, so an address that it computes is the
  one of the copy. The VM does not make pointers from such addresses. The reflect of the program is not native code:
  `unsafe.Pointer(reflect.ValueOf(&x).Pointer())` gives back `x` (the uintptr carries the pointer, also through an
  interface or a method value, and keeps its object alive until the conversion, as gc does in that expression:
  `fixedbugs/issue15329.go`, `testdata/reflectpointer.go`).
- **Memory without an object**: `s := unsafe.Slice((*byte)(unsafe.Pointer(^uintptr(0))), 1)`: a slice needs memory. When the
  result is never used (`_ = unsafe.Slice(p, n)`, `unsafebuiltins.go`), only the checks of gc run (the panics for a nil
  pointer, a negative length, a length past the end of the address space), and nothing is refused: no memory is read
  (`testdata/unsafeunused.go`).
- **Empty strings in an interface**: a Go string of length 0 put in an interface loses its data pointer
  (`runtime.convTstring`), and every VM value is in an interface (`Value.R`). So the VM keeps the data address of an
  empty substring in `Value.N` (`emptySubstring`): `s[5:]` of `"hello"` has the data address of `s`, as in gc, which
  does not move the pointer past the end (`slicecap.go`, `testdata/emptysubdata.go`). The header reads it as a
  number, or as a pointer with no element (`badAddr`). Only a substring keeps it: an empty string put in an
  interface, or returned by native code, has none, as in gc. The data of a slice of cells is its address in the array
  that the program knows (base plus the gc offset, as `uintptr(&a[i])`), also for a slice of capacity 0, whose data
  pointer is the one of the slice it was cut from.
- **Addresses**: `unsafe.Pointer(&s) == unsafe.Pointer(&s.X)` is true in gc, false in the VM
  (a struct and its first field are two separate cells; a conversion of the struct pointer to a pointer to its first
  field type gives the field cell, see above). Addresses are contiguous in the gc sense only inside one
  array the VM knows: an element reached through another slice of the same array (`s := a[1:]`, then
  `&s[0]`) may get another address than `&a[1]`, and a pointer whose array the VM does not know gives the
  real address of its cell. Only arithmetic *that starts from a pointer and goes back to a pointer* is translated.
- **Headers**: a header view refuses a word that does not exist (the capacity of a string). Two views of the same variable
  made by two separate conversions do not share an incomplete header. A header struct built by the program and read as
  a string is a copy for later writes into the struct. `unsafe.Pointer` to `runtime` (`//go:linkname`) is not emulated.
- **Sharing with native code**: interpreted memory is not typed Go memory, except strings and Go `[]byte`. An
  `unsafe.Pointer` to a cell passed to native code that dereferences it reads a `Value`, not the data.

## Why not a real memory

Giving interpreted values a byte representation (like gc) would remove these
limits, but it would change the whole model. Each access would need typed
encoding/decoding. The references (`R`) to strings, maps, closures and native values would need to live
in a separate table. The native boundary through reflect could no longer share
values. The speed gain of the current model comes precisely from `Value`. For Traefik
plugins, the real uses of `unsafe` are mostly zero-copy conversions and
`Sizeof`/`Offsetof`, and these are covered.
