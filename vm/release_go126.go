//go:build !go1.27

package vm

// The behaviors of the standard library of Go 1.26 that the VM implements
// itself (release_go127.go has those of Go 1.27).

// nullValidOnError: (*sql.Null[T]).Scan sets Valid to true before it calls
// the Scan of T, so Valid stays true when that Scan fails.
const nullValidOnError = true
