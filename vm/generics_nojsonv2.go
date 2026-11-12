//go:build !goexperiment.jsonv2 || !go1.27

package vm

// jsonv2Generics: without GOEXPERIMENT=jsonv2 (Go 1.26 by default) there is
// no encoding/json/jsontext, and with Go 1.26 its API is an experiment
// (generics_jsonv2.go).
func jsonv2Generics() map[string]genericImpl { return nil }
