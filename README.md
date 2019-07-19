<p align="center">
<img width="400" src="doc/images/yaegi.png" alt="Yaegi" title="Yaegi" />
</p>

[![Build Status](https://semaphoreci.com/api/v1/projects/8a9d3e41-0a4f-408e-a477-3c9c7418314d/2583906/badge.svg)](https://semaphoreci.com/containous/yaegi)

Yaegi is Another Elegant Go Interpreter. It powers executable Go scripts and plugins, in embedded interpreters or interactive shells, on top of the Go runtime.

## Features

* Complete support of [Go specification][specs]
* In pure Go, using only standard library
* Simple interpreter API: `New()`, `Eval()`, `Use()`
* works everywhere Go works
* All Go & runtime resources accessible from script (with control)
* Security: `unsafe` and `syscall` packages not used or exported by default

## Install

```console
go get -u github.com/containous/yaegi
```

## Usage

### As a command line interpreter

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
	i.Eval(`import "fmt"`)
	i.Eval(`fmt.Println("hello")`)
}
```

### As a dynamic extension framework

The following program is compiled ahead of time, except `bar()` which is interpreted, with the following steps:

1. use of `i.Eval(src)` to evaluate the script in the context of interpreter
2. use of `v, _ := i.Eval("foo.Bar")` to get the symbol from the interpreter context,  as a `reflect.Value`
3. application of `Interface()` method and type assertion to convert `v` into `bar`, as if it was compiled

```go
package main

import "github.com/containous/yaegi/interp"

const src = `package foo
func Bar(s string) string { return s + "-Foo" }`

func main() {
	i := interp.New(interp.Options{})
	i.Eval(src)
	v, _ := i.Eval("foo.Bar")
	bar := v.Interface().(func(string) string)

	r := bar("Kung")
	println(r)
}
```

## Documentation

Documentation about yaegi commands and libraries can be found at usual [godoc.org][docs].

## Contributing

Yaegi is an open source project, and your feedback and contributions are needed and always welcome.

[Issues] and [pull requests] are opened at https://github.com/containous/yaegi

## License

[Apache 2.0][License].

[specs]: https://golang.org/ref/spec
[docs]: https://godoc.org/github.com/containous/yaegi
[license]: https://github.com/containous/yaegi/blob/master/LICENSE
[github]: https://github.com/containous/yaegi
[Issues]: https://github.com/containous/yaegi/issues
[pull requests]: https://github.com/containous/yaegi/issues
