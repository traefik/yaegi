// +build go1.11,!go1.12

package stdlib

// Code generated by 'goexports os/user'. DO NOT EDIT.

import (
	"os/user"
	"reflect"
)

func init() {
	Value["os/user"] = map[string]reflect.Value{
		// function, constant and variable definitions
		"Current":       reflect.ValueOf(user.Current),
		"Lookup":        reflect.ValueOf(user.Lookup),
		"LookupGroup":   reflect.ValueOf(user.LookupGroup),
		"LookupGroupId": reflect.ValueOf(user.LookupGroupId),
		"LookupId":      reflect.ValueOf(user.LookupId),

		// type definitions
		"Group":               reflect.ValueOf((*user.Group)(nil)),
		"UnknownGroupError":   reflect.ValueOf((*user.UnknownGroupError)(nil)),
		"UnknownGroupIdError": reflect.ValueOf((*user.UnknownGroupIdError)(nil)),
		"UnknownUserError":    reflect.ValueOf((*user.UnknownUserError)(nil)),
		"UnknownUserIdError":  reflect.ValueOf((*user.UnknownUserIdError)(nil)),
		"User":                reflect.ValueOf((*user.User)(nil)),

		// interface wrapper definitions

	}
}
