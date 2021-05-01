// Code generated by 'yaegi extract bytes'. DO NOT EDIT.

// +build go1.15,!go1.16

package stdlib

import (
	"bytes"
	"go/constant"
	"go/token"
	"reflect"
)

func init() {
	Symbols["bytes"] = map[string]reflect.Value{
		// default package name identifier
		".name": reflect.ValueOf("bytes"),

		// function, constant and variable definitions
		"Compare":         reflect.ValueOf(bytes.Compare),
		"Contains":        reflect.ValueOf(bytes.Contains),
		"ContainsAny":     reflect.ValueOf(bytes.ContainsAny),
		"ContainsRune":    reflect.ValueOf(bytes.ContainsRune),
		"Count":           reflect.ValueOf(bytes.Count),
		"Equal":           reflect.ValueOf(bytes.Equal),
		"EqualFold":       reflect.ValueOf(bytes.EqualFold),
		"ErrTooLarge":     reflect.ValueOf(&bytes.ErrTooLarge).Elem(),
		"Fields":          reflect.ValueOf(bytes.Fields),
		"FieldsFunc":      reflect.ValueOf(bytes.FieldsFunc),
		"HasPrefix":       reflect.ValueOf(bytes.HasPrefix),
		"HasSuffix":       reflect.ValueOf(bytes.HasSuffix),
		"Index":           reflect.ValueOf(bytes.Index),
		"IndexAny":        reflect.ValueOf(bytes.IndexAny),
		"IndexByte":       reflect.ValueOf(bytes.IndexByte),
		"IndexFunc":       reflect.ValueOf(bytes.IndexFunc),
		"IndexRune":       reflect.ValueOf(bytes.IndexRune),
		"Join":            reflect.ValueOf(bytes.Join),
		"LastIndex":       reflect.ValueOf(bytes.LastIndex),
		"LastIndexAny":    reflect.ValueOf(bytes.LastIndexAny),
		"LastIndexByte":   reflect.ValueOf(bytes.LastIndexByte),
		"LastIndexFunc":   reflect.ValueOf(bytes.LastIndexFunc),
		"Map":             reflect.ValueOf(bytes.Map),
		"MinRead":         reflect.ValueOf(constant.MakeFromLiteral("512", token.INT, 0)),
		"NewBuffer":       reflect.ValueOf(bytes.NewBuffer),
		"NewBufferString": reflect.ValueOf(bytes.NewBufferString),
		"NewReader":       reflect.ValueOf(bytes.NewReader),
		"Repeat":          reflect.ValueOf(bytes.Repeat),
		"Replace":         reflect.ValueOf(bytes.Replace),
		"ReplaceAll":      reflect.ValueOf(bytes.ReplaceAll),
		"Runes":           reflect.ValueOf(bytes.Runes),
		"Split":           reflect.ValueOf(bytes.Split),
		"SplitAfter":      reflect.ValueOf(bytes.SplitAfter),
		"SplitAfterN":     reflect.ValueOf(bytes.SplitAfterN),
		"SplitN":          reflect.ValueOf(bytes.SplitN),
		"Title":           reflect.ValueOf(bytes.Title),
		"ToLower":         reflect.ValueOf(bytes.ToLower),
		"ToLowerSpecial":  reflect.ValueOf(bytes.ToLowerSpecial),
		"ToTitle":         reflect.ValueOf(bytes.ToTitle),
		"ToTitleSpecial":  reflect.ValueOf(bytes.ToTitleSpecial),
		"ToUpper":         reflect.ValueOf(bytes.ToUpper),
		"ToUpperSpecial":  reflect.ValueOf(bytes.ToUpperSpecial),
		"ToValidUTF8":     reflect.ValueOf(bytes.ToValidUTF8),
		"Trim":            reflect.ValueOf(bytes.Trim),
		"TrimFunc":        reflect.ValueOf(bytes.TrimFunc),
		"TrimLeft":        reflect.ValueOf(bytes.TrimLeft),
		"TrimLeftFunc":    reflect.ValueOf(bytes.TrimLeftFunc),
		"TrimPrefix":      reflect.ValueOf(bytes.TrimPrefix),
		"TrimRight":       reflect.ValueOf(bytes.TrimRight),
		"TrimRightFunc":   reflect.ValueOf(bytes.TrimRightFunc),
		"TrimSpace":       reflect.ValueOf(bytes.TrimSpace),
		"TrimSuffix":      reflect.ValueOf(bytes.TrimSuffix),

		// type definitions
		"Buffer": reflect.ValueOf((*bytes.Buffer)(nil)),
		"Reader": reflect.ValueOf((*bytes.Reader)(nil)),
	}
}
