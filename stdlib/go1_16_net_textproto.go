// Code generated by 'yaegi extract net/textproto'. DO NOT EDIT.

// +build go1.16

package stdlib

import (
	"net/textproto"
	"reflect"
)

func init() {
	Symbols["net/textproto"] = map[string]reflect.Value{
		// default package name identifier
		".name": reflect.ValueOf("textproto"),

		// function, constant and variable definitions
		"CanonicalMIMEHeaderKey": reflect.ValueOf(textproto.CanonicalMIMEHeaderKey),
		"Dial":                   reflect.ValueOf(textproto.Dial),
		"NewConn":                reflect.ValueOf(textproto.NewConn),
		"NewReader":              reflect.ValueOf(textproto.NewReader),
		"NewWriter":              reflect.ValueOf(textproto.NewWriter),
		"TrimBytes":              reflect.ValueOf(textproto.TrimBytes),
		"TrimString":             reflect.ValueOf(textproto.TrimString),

		// type definitions
		"Conn":          reflect.ValueOf((*textproto.Conn)(nil)),
		"Error":         reflect.ValueOf((*textproto.Error)(nil)),
		"MIMEHeader":    reflect.ValueOf((*textproto.MIMEHeader)(nil)),
		"Pipeline":      reflect.ValueOf((*textproto.Pipeline)(nil)),
		"ProtocolError": reflect.ValueOf((*textproto.ProtocolError)(nil)),
		"Reader":        reflect.ValueOf((*textproto.Reader)(nil)),
		"Writer":        reflect.ValueOf((*textproto.Writer)(nil)),
	}
}
