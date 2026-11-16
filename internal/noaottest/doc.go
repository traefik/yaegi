// Package noaottest holds tests of the vm package in a binary without the
// AOT copies (it does not import github.com/traefik/yaegi/v2/aot/all), as a
// host that forgot that import. The tests of package vm cannot do it: their
// binary imports aot/all.
package noaottest
