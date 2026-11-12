// Package minicfg is a small reflect-based config loader, standing for the
// many third-party libraries plugins use that are not in yaegi v2's AOT list.
package minicfg

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// Validator is implemented by config types that check themselves.
type Validator interface{ Validate() error }

// Load fills dst (a pointer to struct) from kv using `env:"KEY"` tags,
// then calls Defaults() by name if present and Validate() if implemented.
func Load(dst any, kv map[string]string) error {
	v := reflect.ValueOf(dst)
	if v.Kind() != reflect.Pointer || v.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("minicfg: want pointer to struct, got %T", dst)
	}
	if err := fill(v.Elem(), kv, ""); err != nil {
		return err
	}
	if m := v.MethodByName("Defaults"); m.IsValid() {
		m.Call(nil)
	}
	if val, ok := dst.(Validator); ok {
		return val.Validate()
	}
	return nil
}

func fill(v reflect.Value, kv map[string]string, prefix string) error {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		key := f.Tag.Get("env")
		fv := v.Field(i)
		if f.Type.Kind() == reflect.Struct {
			if err := fill(fv, kv, prefix+key); err != nil {
				return err
			}
			continue
		}
		s, ok := kv[prefix+key]
		if key == "" || !ok {
			continue
		}
		if u, ok := fv.Addr().Interface().(interface{ UnmarshalText([]byte) error }); ok {
			if err := u.UnmarshalText([]byte(s)); err != nil {
				return err
			}
			continue
		}
		switch fv.Kind() {
		case reflect.String:
			fv.SetString(s)
		case reflect.Int, reflect.Int64:
			n, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				return fmt.Errorf("%s: %w", f.Name, err)
			}
			fv.SetInt(n)
		case reflect.Bool:
			b, _ := strconv.ParseBool(s)
			fv.SetBool(b)
		case reflect.Slice:
			parts := strings.Split(s, ",")
			sl := reflect.MakeSlice(fv.Type(), len(parts), len(parts))
			for j, p := range parts {
				sl.Index(j).SetString(p)
			}
			fv.Set(sl)
		}
	}
	return nil
}

// Describe returns "Type{field=value,...}" using the type name and fields.
func Describe(x any) string {
	v := reflect.Indirect(reflect.ValueOf(x))
	var b strings.Builder
	b.WriteString(v.Type().Name() + "{")
	for i := 0; i < v.NumField(); i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, "%s=%v", v.Type().Field(i).Name, v.Field(i).Interface())
	}
	b.WriteString("}")
	return b.String()
}
