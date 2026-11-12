// Package stringslite is the aotgen shim of internal/stringslite: the
// original depends on internal/bytealg (assembly), which cannot be copied
// out of the standard library. This version delegates to package strings.
package stringslite

import "strings"

func HasPrefix(s, prefix string) bool                        { return strings.HasPrefix(s, prefix) }
func HasSuffix(s, suffix string) bool                        { return strings.HasSuffix(s, suffix) }
func IndexByte(s string, c byte) int                         { return strings.IndexByte(s, c) }
func Index(s, substr string) int                             { return strings.Index(s, substr) }
func Cut(s, sep string) (before, after string, found bool)   { return strings.Cut(s, sep) }
func CutPrefix(s, prefix string) (after string, found bool)  { return strings.CutPrefix(s, prefix) }
func CutSuffix(s, suffix string) (before string, found bool) { return strings.CutSuffix(s, suffix) }
func TrimPrefix(s, prefix string) string                     { return strings.TrimPrefix(s, prefix) }
func TrimSuffix(s, suffix string) string                     { return strings.TrimSuffix(s, suffix) }
func Clone(s string) string                                  { return strings.Clone(s) }
