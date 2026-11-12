# Tests of the native boundary

Each program of this directory tests one package of the standard library
(or a part of a big one): it gives values of the program's own types to the
functions of the package, where the package changes what it does because of
the dynamic type or the methods of the value. The places come from the
inventory, [docs/BOUNDARY.md](../../docs/BOUNDARY.md), made by
`cmd/boundaryscan`.

`TestBoundary` (cmd/runtests) runs each program with `go run`, and with
yaegi v2 in VM mode and in AOT mode (`-aot`), and compares the output of each
probe. `TestBoundaryInventory` checks that every row of the inventory has a
probe.

## The form of a program

- The file is named after the package: `io.go`, `encoding_json.go`,
  `net_http.go` (a big package may have more: `net_http_2.go`).
- It has the two helpers below, and calls `probe("id", func() { ... })` for
  each probe. Every line of a probe starts with its id (use `out`), and a
  panic of a probe is recovered and printed: one probe that differs does
  not hide the others.

  ```go
  var cur string

  // probe runs one probe: its lines start with its id, and its panic is printed.
  func probe(id string, f func()) {
  	cur = id
  	defer func() {
  		if r := recover(); r != nil {
  			fmt.Printf("%s: panic: %T: %v\n", id, r, r)
  		}
  	}()
  	f()
  }

  // out prints one line of the current probe.
  func out(format string, args ...any) { fmt.Printf(cur+": "+format+"\n", args...) }
  ```

- Before each probe, one comment line per row of the inventory that it
  tests:

  ```go
  // row: io Copy,CopyBuffer,CopyN assert x.(io.WriterTo)
  probe("Copy/WriterTo", func() { ... })
  ```

  The fields are the package path, the functions (as in the inventory:
  `F`, `T.M`, `(*T).M`, separated by commas), the kind (`assert`,
  `reflect`, `panic`) and the check, as in the inventory. The check (or the
  kind) may be `*`: the probe then tests every check of these functions.
  Use `*` only when the probe really gives the values that each check looks
  at.

- A probe gives the function values of program types with the method and
  without it, and in the forms that the parameter accepts: a value of a type
  with value receivers, a pointer to a type with pointer receivers, the
  value inside an interface variable, inside `[]any`, `map[string]any` and a
  struct field (when the parameter is `any`), and an instance of a generic
  type. For `x.(T), T unnamed` rows, give a named type of the program over
  `T` (`type S string`): Go must not take the path of `T`. For `panic` rows,
  make the panic happen and print it. For `reflect` rows, print what the
  function prints or returns for the type (its name, its kind...).

- Print the results, never addresses, times, random numbers or map orders
  (sort the keys). No network, no files outside `os.MkdirTemp` (and remove
  them), no environment. The program must end with exit status 0 under
  `go run`, in a few seconds.

- Only the packages that a restricted plugin can import.

## Probes that do not give the output of Go yet

`known.tsv` lists them: `file:id`, the mode (`vm` or `aot`, or `vm@go1.27`
for a probe that differs only with the standard library of one Go
release of the support window), the class
(`wrong`: a silent wrong result, `crash`: an error of the VM or a wrong
panic, `message`: a different panic message or panic value), the issue (a
short name of the cause, such as `named-type` or `type-name`) and why.
`TestBoundary` fails when such a probe gives the output of Go: remove its
line then. Never add a line without a known cause.
