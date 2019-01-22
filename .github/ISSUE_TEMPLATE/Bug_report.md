---
name: Bug report
about: Create a report to help us improve

---

The following program `sample.go` triggers a panic:

```go
package main

func main() {
// add a sample
}
```

Expected result:
```console
$ go run ./sample.go
// ouput
```

Got:
```console
$ dyngo ./sample.go
// ouput
```
