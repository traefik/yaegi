package stdlib

// Code generated by 'goexports path/filepath'. DO NOT EDIT.

import (
	"path/filepath"
	"reflect"
)

func init() {
	Value["path/filepath"] = map[string]reflect.Value{
		"Abs":           reflect.ValueOf(filepath.Abs),
		"Base":          reflect.ValueOf(filepath.Base),
		"Clean":         reflect.ValueOf(filepath.Clean),
		"Dir":           reflect.ValueOf(filepath.Dir),
		"ErrBadPattern": reflect.ValueOf(filepath.ErrBadPattern),
		"EvalSymlinks":  reflect.ValueOf(filepath.EvalSymlinks),
		"Ext":           reflect.ValueOf(filepath.Ext),
		"FromSlash":     reflect.ValueOf(filepath.FromSlash),
		"Glob":          reflect.ValueOf(filepath.Glob),
		"HasPrefix":     reflect.ValueOf(filepath.HasPrefix),
		"IsAbs":         reflect.ValueOf(filepath.IsAbs),
		"Join":          reflect.ValueOf(filepath.Join),
		"ListSeparator": reflect.ValueOf(filepath.ListSeparator),
		"Match":         reflect.ValueOf(filepath.Match),
		"Rel":           reflect.ValueOf(filepath.Rel),
		"Separator":     reflect.ValueOf(filepath.Separator),
		"SkipDir":       reflect.ValueOf(filepath.SkipDir),
		"Split":         reflect.ValueOf(filepath.Split),
		"SplitList":     reflect.ValueOf(filepath.SplitList),
		"ToSlash":       reflect.ValueOf(filepath.ToSlash),
		"VolumeName":    reflect.ValueOf(filepath.VolumeName),
		"Walk":          reflect.ValueOf(filepath.Walk),
	}

	Type["path/filepath"] = map[string]reflect.Type{
		"WalkFunc": reflect.TypeOf((*filepath.WalkFunc)(nil)).Elem(),
	}
}
