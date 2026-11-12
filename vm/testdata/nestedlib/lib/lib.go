// Package lib is a nested module in the directory of the root module. Go
// does not read it: the vendored copy wins.
package lib

func Name() string { return "root lib directory" }
