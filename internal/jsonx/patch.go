package jsonx

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ApplyPatch applies an RFC 6902 JSON patch to the target JSON blob.
//
// Test operations are not supported.
func ApplyPatch(target []byte, patchPath string) ([]byte, error) {
	f, err := os.Open(patchPath)
	if err != nil {
		return nil, fmt.Errorf("read (patch): %v", err)
	}

	var p Patch
	err = json.NewDecoder(f).Decode(&p)
	if err != nil {
		return nil, fmt.Errorf("json (patch): %v", err)
	}

	var v interface{}
	err = json.Unmarshal(target, &v)
	if err != nil {
		return nil, fmt.Errorf("json (target): %v", err)
	}

	for i, p := range p {
		err = p.Apply(&v)
		if err != nil {
			return nil, fmt.Errorf("patch [%d]: %v", i, err)
		}
	}

	target, err = json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("json (marshal): %v", err)
	}

	return target, nil
}

// PatchOpType is an RFC 6902 JSON patch operation type.
type PatchOpType string

const (
	// PatchOpAdd is an RFC 6902 JSON patch add operation.
	PatchOpAdd PatchOpType = "add"
	// PatchOpRemove is an RFC 6902 JSON patch remove operation.
	PatchOpRemove PatchOpType = "remove"
	// PatchOpReplace is an RFC 6902 JSON patch replace operation.
	PatchOpReplace PatchOpType = "replace"
	// PatchOpCopy is an RFC 6902 JSON patch copy operation.
	PatchOpCopy PatchOpType = "copy"
	// PatchOpMove is an RFC 6902 JSON patch move operation.
	PatchOpMove PatchOpType = "move"
)

// NeedsPath returns true if the operation must include "path".
func (op PatchOpType) NeedsPath() bool { return true }

// NeedsFrom returns true if the operation must include "from".
func (op PatchOpType) NeedsFrom() bool {
	switch op {
	case PatchOpCopy, PatchOpMove:
		return true
	default:
		return false
	}
}

// PatchPointer is an RFC 6901 JSON pointer.
type PatchPointer string

// Parse parses the pointer, returning an array of names/indices.
func (p PatchPointer) Parse() ([]string, error) {
	if len(p) == 0 {
		return nil, errors.New("pointer is empty")
	} else if p[0] != '/' {
		return nil, errors.New("pointer does not start with '/'")
	}

	s := strings.Split(string(p[1:]), "/")
	for i := range s {
		s[i] = strings.ReplaceAll(s[i], "~1", "/")
		s[i] = strings.ReplaceAll(s[i], "~0", "~")
	}
	return s, nil
}

// PatchOp is an RFC 6902 JSON patch operation.
type PatchOp struct {
	Op    PatchOpType
	From  PatchPointer
	Path  PatchPointer
	Value interface{}
}

// Patch is an RFC 6902 JSON patch.
type Patch []PatchOp

// Apply applies the patch. Apply returns ErrNotFound if From or Path are
// required and refer to a value that does not exist in v.
func (op *PatchOp) Apply(v *interface{}) error {
	path, err := op.Path.Parse()
	if err != nil {
		return fmt.Errorf("path: %w", err)
	}

	from, err := op.From.Parse()
	if op.Op.NeedsFrom() && err != nil {
		return fmt.Errorf("from: %w", err)
	}
	_ = from

	var jv jsonValue = ptrValue{v}
	switch op.Op {
	case PatchOpAdd:
		jv, err := jsonDerefAll(jv, path)
		if err != nil && (jv == nil || !errors.Is(err, ErrNotFound)) {
			return err
		}
		jv.Set(op.Value)

	case PatchOpRemove:
		jv, err := jsonDerefAll(jv, path)
		if err != nil {
			return err
		}
		jv.Delete()

	case PatchOpReplace:
		jv, err := jsonDerefAll(jv, path)
		if err != nil {
			return err
		}
		jv.Delete()
		jv.Set(op.Value)

	case PatchOpCopy:
		ju, err := jsonDerefAll(jv, from)
		if err != nil {
			return err
		}
		jv, err := jsonDerefAll(jv, path)
		if err != nil && (jv == nil || !errors.Is(err, ErrNotFound)) {
			return err
		}
		jv.Set(ju.Get())

	case PatchOpMove:
		ju, err := jsonDerefAll(jv, from)
		if err != nil {
			return err
		}
		jv, err := jsonDerefAll(jv, path)
		if err != nil && (jv == nil || !errors.Is(err, ErrNotFound)) {
			return err
		}
		v := ju.Get()
		ju.Delete()
		jv.Set(v)

	default:
		return fmt.Errorf("unsupported patch operation %q", op.Op)
	}

	return nil
}

// ErrNotFound indicates that the specified node was not found.
var ErrNotFound = errors.New("not found")

type jsonValue interface {
	Get() interface{}
	Set(interface{})
	Delete()
}

type ptrValue struct {
	v *interface{}
}

func (v ptrValue) Get() interface{}  { return *v.v }
func (v ptrValue) Set(u interface{}) { *v.v = u }
func (v ptrValue) Delete()           { *v.v = nil }

type listEntry struct {
	v jsonValue
	i int
}

func (e listEntry) Get() interface{} {
	return e.v.Get().([]interface{})[e.i]
}

func (e listEntry) Set(u interface{}) {
	v := e.v.Get().([]interface{})
	switch {
	case e.i < 0:
		v = append(v, u)
	case len(v) < cap(v):
		v = v[:len(v)+1]
		for i := len(v) - 1; i > e.i; i-- {
			v[i] = v[i-1]
		}
	default:
		v2 := make([]interface{}, 0, len(v)+1)
		v2 = append(v2, v[:e.i]...)
		v2 = append(v2, u)
		v2 = append(v2, v[e.i:]...)
		v = v2
	}

	e.v.Set(v)
}

func (e listEntry) Delete() {
	v := e.v.Get().([]interface{})
	if e.i < 0 {
		v = v[:len(v)-1]
	} else {
		v = append(v[:e.i], v[e.i+1:]...)
	}
	e.v.Set(v)
}

type mapEntry struct {
	v jsonValue
	i string
}

func (e mapEntry) Get() interface{} {
	return e.v.Get().(map[string]interface{})[e.i]
}

func (e mapEntry) Set(u interface{}) {
	v := e.v.Get().(map[string]interface{})
	if v == nil {
		v = map[string]interface{}{}
		e.v.Set(v)
	}
	v[e.i] = u
}

func (e mapEntry) Delete() {
	delete(e.v.Get().(map[string]interface{}), e.i)
}

func jsonDeref(v jsonValue, path, fullPath string) (jsonValue, error) {
	errorf := func(format string, args ...interface{}) error {
		err := fmt.Errorf(format, args...)
		if fullPath == "" {
			return err
		}
		return fmt.Errorf("%s: %w", fullPath, err)
	}

	switch u := v.Get().(type) {
	case map[string]interface{}:
		if _, ok := u[path]; ok {
			return mapEntry{v, path}, nil
		}
		return mapEntry{v, path}, errorf("%q %w", path, ErrNotFound)

	case []interface{}:
		if path == "-" {
			return listEntry{v, -1}, errorf("%q %w", path, ErrNotFound)
		}
		i, err := strconv.ParseUint(path, 10, 32)
		if err != nil {
			return nil, errorf("invalid array index: %w", err)
		}
		if int(i) <= len(u) {
			return listEntry{v, int(i)}, nil
		}
		return listEntry{v, int(i)}, errorf("%q %w", path, ErrNotFound)

	default:
		return nil, errorf("%T cannot be indexed", v)
	}
}

func jsonDerefAll(v jsonValue, path []string) (jsonValue, error) {
	var fullPath string
	var err error
	for i, p := range path {
		v, err = jsonDeref(v, p, fullPath)
		if err != nil {
			if v == nil || i < len(path)-1 || !errors.Is(err, ErrNotFound) {
				return nil, err
			}
		}
		path, fullPath = path[1:], fullPath+"/"+path[0]
	}
	return v, err
}
