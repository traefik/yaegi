// +build go1.12, !go1.13

package stdlib

// Code generated by 'goexports net/smtp'. DO NOT EDIT.

import (
	"net/smtp"
	"reflect"
)

func init() {
	Value["net/smtp"] = map[string]reflect.Value{
		// function, constant and variable definitions
		"CRAMMD5Auth": reflect.ValueOf(smtp.CRAMMD5Auth),
		"Dial":        reflect.ValueOf(smtp.Dial),
		"NewClient":   reflect.ValueOf(smtp.NewClient),
		"PlainAuth":   reflect.ValueOf(smtp.PlainAuth),
		"SendMail":    reflect.ValueOf(smtp.SendMail),

		// type definitions
		"Auth":       reflect.ValueOf((*smtp.Auth)(nil)),
		"Client":     reflect.ValueOf((*smtp.Client)(nil)),
		"ServerInfo": reflect.ValueOf((*smtp.ServerInfo)(nil)),
	}
}
