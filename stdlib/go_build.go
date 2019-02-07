package stdlib

// Code generated by 'goexports go/build'. DO NOT EDIT.

import (
	"go/build"
	"reflect"
)

func init() {
	Value["go/build"] = map[string]reflect.Value{
		"AllowBinary":   reflect.ValueOf(build.AllowBinary),
		"ArchChar":      reflect.ValueOf(build.ArchChar),
		"Default":       reflect.ValueOf(&build.Default).Elem(),
		"FindOnly":      reflect.ValueOf(build.FindOnly),
		"IgnoreVendor":  reflect.ValueOf(build.IgnoreVendor),
		"Import":        reflect.ValueOf(build.Import),
		"ImportComment": reflect.ValueOf(build.ImportComment),
		"ImportDir":     reflect.ValueOf(build.ImportDir),
		"IsLocalImport": reflect.ValueOf(build.IsLocalImport),
		"ToolDir":       reflect.ValueOf(&build.ToolDir).Elem(),
	}

	Type["go/build"] = map[string]reflect.Type{
		"Context":              reflect.TypeOf((*build.Context)(nil)).Elem(),
		"ImportMode":           reflect.TypeOf((*build.ImportMode)(nil)).Elem(),
		"MultiplePackageError": reflect.TypeOf((*build.MultiplePackageError)(nil)).Elem(),
		"NoGoError":            reflect.TypeOf((*build.NoGoError)(nil)).Elem(),
		"Package":              reflect.TypeOf((*build.Package)(nil)).Elem(),
	}
}
