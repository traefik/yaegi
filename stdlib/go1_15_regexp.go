// Code generated by 'github.com/traefik/yaegi/extract regexp'. DO NOT EDIT.

// +build go1.15,!go1.16

package stdlib

import (
	"reflect"
	"regexp"
)

func init() {
	Symbols["regexp"] = map[string]reflect.Value{
		// function, constant and variable definitions
		"Compile":          reflect.ValueOf(regexp.Compile),
		"CompilePOSIX":     reflect.ValueOf(regexp.CompilePOSIX),
		"Match":            reflect.ValueOf(regexp.Match),
		"MatchReader":      reflect.ValueOf(regexp.MatchReader),
		"MatchString":      reflect.ValueOf(regexp.MatchString),
		"MustCompile":      reflect.ValueOf(regexp.MustCompile),
		"MustCompilePOSIX": reflect.ValueOf(regexp.MustCompilePOSIX),
		"QuoteMeta":        reflect.ValueOf(regexp.QuoteMeta),

		// type definitions
		"Regexp": reflect.ValueOf((*regexp.Regexp)(nil)),
	}
}
