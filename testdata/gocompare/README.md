# testdata/gocompare: programs compared with Go in every mode

Each `*.go` file here is a small program, in the directory of its area
(`ints/`, `strings/`, `maps/`...). `TestGoCompare` (`cmd/runtests`) builds
it with `go build`, runs it, and runs it with yaegi v2 in each mode (default,
`-aot`, `-opaque`, `-restricted`): the output (stdout and stderr together)
must be the same. A new file is a new test, with no list to update.

These programs are the safety net of the changes to the VM, also the ones
made by AI: a change that makes yaegi v2 differ from Go on one of them fails CI.
So each program checks many cases, prints each result, and the more of them
the better.

Rules of a program (`TestGoCompareRules` checks the first three):

- `package main`, one program per file, `func main` in it.
- It starts with a comment that says what it checks.
- It is in the directory of its area, never at the top of `gocompare/`.
- It exits with status 0: recover the panics it checks and print the
  recovered value. A panic that is not recovered prints the stacks of the
  goroutines, which differ between Go and yaegi v2.
- Its output is the same at each run, on every platform (CI runs these
  tests on linux/amd64, linux/arm64 and linux/386 today; the other
  platforms of CI, darwin, windows, arm and s390x, may run them later):
  - no map printed in range order (fmt prints maps sorted; sort the keys of
    a range);
  - no address, no `%p`, no time of day, no random number without a fixed
    seed, no output that depends on the scheduler (goroutines print through
    a channel or after a `sync.WaitGroup`, in a fixed order);
  - the same results with a 32-bit `int` (linux/386, linux/arm): use
    `int64` and `uint64` for large values;
  - no float expression that gc may fuse: on arm64, ppc64le and s390x, gc
    computes `x*y + z` (also across statements) with one FMA instruction,
    rounded once, as the spec allows, and the result differs from the
    platforms without it. Round each product with a conversion:
    `float64(x*y) + z`;
  - no file system, network, environment or process (paths, PIDs, the
    working directory).
- It only imports packages that a restricted plugin may import: no `os/exec`,
  `unsafe`, `syscall`, `net/http/pprof`, `plugin`, `go/types`...
- It runs in less than a second with Go.

Run the programs of one area:

```bash
go test -count=1 -run 'TestGoCompare/ints/' ./cmd/runtests
```

A program that shows a difference between Go and yaegi v2 is a bug of the VM:
fix the VM (the program is then its regression test), or open an issue with
the program. Never change a program so that it hides a difference.
