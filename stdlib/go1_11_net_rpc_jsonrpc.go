// +build go1.11,!go1.12

package stdlib

// Code generated by 'goexports net/rpc/jsonrpc'. DO NOT EDIT.

import (
	"net/rpc/jsonrpc"
	"reflect"
)

func init() {
	Value["net/rpc/jsonrpc"] = map[string]reflect.Value{
		// function, constant and variable definitions
		"Dial":           reflect.ValueOf(jsonrpc.Dial),
		"NewClient":      reflect.ValueOf(jsonrpc.NewClient),
		"NewClientCodec": reflect.ValueOf(jsonrpc.NewClientCodec),
		"NewServerCodec": reflect.ValueOf(jsonrpc.NewServerCodec),
		"ServeConn":      reflect.ValueOf(jsonrpc.ServeConn),

		// type definitions

		// interface wrapper definitions

	}
}
