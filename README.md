<p align="center">
<img width="400" src="doc/images/yaegi.png" alt="Yaegi" title="Yaegi" />
</p>

[![release](https://img.shields.io/github/tag-date/containous/yaegi.svg?label=alpha)](https://github.com/containous/yaegi/releases)
[![Build Status](https://travis-ci.com/containous/yaegi.svg?branch=master)](https://travis-ci.com/containous/yaegi)
[![GoDoc](https://godoc.org/github.com/containous/yaegi?status.svg)](https://godoc.org/github.com/containous/yaegi)
[![Discourse status](https://img.shields.io/discourse/https/community.containo.us/status?label=Community&style=social)](https://community.containo.us/c/yaegi)

Yaegi is Another Elegant Go Interpreter.
It powers executable Go scripts and plugins, in embedded interpreters or interactive shells, on top of the Go runtime.

## Features

* Complete support of [Go specification][specs]
* Written in pure Go, using only the standard library
* Simple interpreter API: `New()`, `Eval()`, `Use()`
* Works everywhere Go works
* All Go & runtime resources accessible from script (with control)
* Security: `unsafe` and `syscall` packages neither used nor exported by default
* Support Go 1.12 and Go 1.13 (the latest 2 major releases)

## Install

### Go package

```go
import "github.com/containous/yaegi/interp"
```

### Command-line executable

```bash
go get -u github.com/containous/yaegi/cmd/yaegi
```

Note that you can use [rlwrap](https://github.com/hanslub42/rlwrap) (install with your favorite package manager),
and alias the `yaegi` command in `alias yaegi='rlwrap yaegi'` in your `~/.bashrc`, to have history and command line edition.

## Usage

### As an embedded interpreter

Create an interpreter with `New()`, run Go code with `Eval()`:

```go
package main

import (
	"github.com/containous/yaegi/interp"
	"github.com/containous/yaegi/stdlib"
)

func main() {
	i := interp.New(interp.Options{})

	i.Use(stdlib.Symbols)

	_, err := i.Eval(`import "fmt"`)
	if err != nil {
		panic(err)
	}

	_, err = i.Eval(`fmt.Println("Hello Yaegi")`)
	if err != nil {
		panic(err)
	}
}
```

[Go Playground](https://play.golang.org/p/zzvw4VlerLP)

### As a dynamic extension framework

The following program is compiled ahead of time, except `bar()` which is interpreted, with the following steps:

1. use of `i.Eval(src)` to evaluate the script in the context of interpreter
2. use of `v, err := i.Eval("foo.Bar")` to get the symbol from the interpreter context,  as a `reflect.Value`
3. application of `Interface()` method and type assertion to convert `v` into `bar`, as if it was compiled

```go
package main

import "github.com/containous/yaegi/interp"

const src = `package foo
func Bar(s string) string { return s + "-Foo" }`

func main() {
	i := interp.New(interp.Options{})

	_, err := i.Eval(src)
	if err != nil {
		panic(err)
	}

	v, err := i.Eval("foo.Bar")
	if err != nil {
		panic(err)
	}

	bar := v.Interface().(func(string) string)

	r := bar("Kung")
	println(r)
}
```

[Go Playground](https://play.golang.org/p/6SEAoaO7n0U)

### As a command-line interpreter

The Yaegi command can run an interactive Read-Eval-Print-Loop:

```console
$ yaegi
> 1 + 2
3
> import "fmt"
> fmt.Println("Hello World")
Hello World
>
```

Or interpret Go files:

```console
$ yaegi cmd/yaegi/yaegi.go
>
```

Or for Go scripting in the shebang line:

```console
$ cat /tmp/test
#!/usr/bin/env yaegi
package main

import "fmt"

func main() {
	fmt.Println("test")
}
$ ls -la /tmp/test
-rwxr-xr-x 1 dow184 dow184 93 Jan  6 13:38 /tmp/test
$ /tmp/test
test
```

## Documentation

Documentation about Yaegi commands and libraries can be found at usual [godoc.org][docs].

## Limitations

Beside the known [bugs] which are supposed to be fixed in the short term, there are some limitations not planned to be addressed soon:

- assembly files (`.s`) are not supported
- calling C code is not supported (no virtual "C" package)
- interfaces to be used from the pre-compiled code can not be added dynamically, as it is required to pre-compile interface wrappers
- representation of types by `reflect` and printing values using %T may give different results between compiled mode and interpreted mode
- interpreting computation intensive code is likely to remain significantly slower than in compiled mode

## Contributing

[Contributing guide](CONTRIBUTING.md).

## License

[Apache 2.0][License].

[specs]: https://golang.org/ref/spec
[docs]: https://godoc.org/github.com/containous/yaegi
[license]: https://github.com/containous/yaegi/blob/master/LICENSE
[github]: https://github.com/containous/yaegi
[bugs]: https://github.com/containous/yaegi/issues?q=is%3Aissue+is%3Aopen+label%3Abug
