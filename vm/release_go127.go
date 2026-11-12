//go:build go1.27

package vm

// The behaviors of the standard library of Go 1.27 that the VM implements
// itself (release_go126.go has those of Go 1.26).

// nullValidOnError: (*sql.Null[T]).Scan sets Valid to err == nil, so Valid
// is false when the Scan of T fails.
const nullValidOnError = false
