// Package lib is the source of the root module. The nested module plugin
// vendors another copy of it, and that copy must win.
package lib

func Name() string { return "root module source" }
