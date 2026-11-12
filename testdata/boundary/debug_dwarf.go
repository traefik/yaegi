// Native boundary of debug/dwarf: the Val of the fields of an Entry are
// read with x.(int64), x.(uint64) and x.(string); a program can build an
// Entry with values of its own types (docs/BOUNDARY.md).
package main

import (
	"debug/dwarf"
	"encoding/binary"
	"fmt"
)

var cur string

// probe runs one probe: its lines start with its id, and its panic is printed.
func probe(id string, f func()) {
	cur = id
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("%s: panic: %T: %v\n", id, r, r)
		}
	}()
	f()
}

// out prints one line of the current probe.
func out(format string, args ...any) { fmt.Printf(cur+": "+format+"\n", args...) }

type (
	myInt64  int64
	myUint64 uint64
	myString string
)

// abbrev: 1 = compile unit with children (name, low_pc, high_pc as data8,
// addr_base), 2 = base type (name, encoding, byte_size).
var abbrev = []byte{
	1, 0x11, 1,
	0x03, 0x08, 0x11, 0x01, 0x12, 0x07, 0x73, 0x17, 0, 0,
	2, 0x24, 0,
	0x03, 0x08, 0x3e, 0x0b, 0x0b, 0x0b, 0, 0,
	0,
}

// info is a DWARF 5 compile unit with one base type "int".
func info() []byte {
	var body []byte
	body = append(body, 5, 0)       // version 5
	body = append(body, 1, 8)       // DW_UT_compile, address size 8
	body = append(body, 0, 0, 0, 0) // abbrev offset
	body = append(body, 1)          // compile unit (offset 12)
	body = append(body, "cu.go\x00"...)
	body = binary.LittleEndian.AppendUint64(body, 0x1000) // low_pc
	body = binary.LittleEndian.AppendUint64(body, 0x40)   // high_pc (length)
	body = binary.LittleEndian.AppendUint32(body, 8)      // addr_base
	body = append(body, 2)                                // base type (offset 39)
	body = append(body, "int\x00"...)
	body = append(body, 5, 8) // signed, 8 bytes
	body = append(body, 0)    // end of children
	return append(binary.LittleEndian.AppendUint32(nil, uint32(len(body))), body...)
}

// line is not a valid line table: the reader reports an error when it
// reads it.
var line = []byte{0xff, 0xff, 0xff, 0xff, 1, 2, 3}

func newData() *dwarf.Data {
	d, err := dwarf.New(abbrev, nil, nil, info(), line, nil, nil, nil)
	if err != nil {
		out("New: %v", err)
		return nil
	}
	return d
}

func entryString(e *dwarf.Entry) string {
	if e == nil {
		return "nil"
	}
	s := fmt.Sprintf("@%d %v children=%v", e.Offset, e.Tag, e.Children)
	for _, f := range e.Field {
		s += fmt.Sprintf(" %v=%v(%T,%v)", f.Attr, f.Val, f.Val, f.Class)
	}
	return s
}

func main() {
	// row: debug/dwarf (*Data).Reader,(*Reader).Next,(*Reader).Seek,(*Reader).SkipChildren,(*Data).Type assert x.(T), T unnamed
	probe("Reader", func() {
		d := newData()
		if d == nil {
			return
		}
		r := d.Reader()
		for {
			e, err := r.Next()
			if err != nil || e == nil {
				out("end: %v", err)
				break
			}
			out("Next: %s", entryString(e))
		}
		r.Seek(39)
		e, err := r.Next()
		out("Seek 39: %s %v", entryString(e), err)
		r.Seek(0)
		e, _ = r.Next()
		r.SkipChildren()
		e, err = r.Next()
		out("SkipChildren: %s %v", entryString(e), err)
		t, err := d.Type(39)
		out("Type: %T %v %v", t, t, err)
		if t != nil {
			out("Type size: %d", t.Size())
		}
	})
	// row: debug/dwarf (*Data).Ranges,(*Reader).SeekPC assert x.(T), T unnamed
	probe("Ranges", func() {
		d := newData()
		if d == nil {
			return
		}
		r := d.Reader()
		cu, _ := r.Next()
		rs, err := d.Ranges(cu)
		out("native: %v %v", rs, err)
		for _, pc := range []uint64{0x1000, 0x1010, 0x2000} {
			e, err := d.Reader().SeekPC(pc)
			out("SeekPC %#x: %s %v", pc, entryString(e), err)
		}
		entries := []*dwarf.Entry{
			{Offset: cu.Offset, Tag: dwarf.TagSubprogram, Field: []dwarf.Field{
				{Attr: dwarf.AttrLowpc, Val: uint64(0x10), Class: dwarf.ClassAddress},
				{Attr: dwarf.AttrHighpc, Val: uint64(0x20), Class: dwarf.ClassAddress},
			}},
			{Offset: cu.Offset, Tag: dwarf.TagSubprogram, Field: []dwarf.Field{
				{Attr: dwarf.AttrLowpc, Val: myUint64(0x10), Class: dwarf.ClassAddress},
				{Attr: dwarf.AttrHighpc, Val: uint64(0x20), Class: dwarf.ClassAddress},
			}},
			{Offset: cu.Offset, Tag: dwarf.TagSubprogram, Field: []dwarf.Field{
				{Attr: dwarf.AttrLowpc, Val: uint64(0x10), Class: dwarf.ClassAddress},
				{Attr: dwarf.AttrHighpc, Val: myUint64(0x20), Class: dwarf.ClassAddress},
			}},
			{Offset: cu.Offset, Tag: dwarf.TagSubprogram, Field: []dwarf.Field{
				{Attr: dwarf.AttrLowpc, Val: uint64(0x10), Class: dwarf.ClassAddress},
				{Attr: dwarf.AttrHighpc, Val: int64(8), Class: dwarf.ClassConstant},
			}},
			{Offset: cu.Offset, Tag: dwarf.TagSubprogram, Field: []dwarf.Field{
				{Attr: dwarf.AttrLowpc, Val: uint64(0x10), Class: dwarf.ClassAddress},
				{Attr: dwarf.AttrHighpc, Val: myInt64(8), Class: dwarf.ClassConstant},
			}},
		}
		for i, e := range entries {
			rs, err := d.Ranges(e)
			out("program entry %d: %v %v", i, rs, err)
		}
	})
	// row: debug/dwarf (*Data).LineReader assert x.(T), T unnamed
	probe("LineReader", func() {
		d := newData()
		if d == nil {
			return
		}
		cu, _ := d.Reader().Next()
		lr, err := d.LineReader(cu)
		out("native cu: %v %v", lr != nil, err)
		entries := []*dwarf.Entry{
			{Offset: cu.Offset, Tag: dwarf.TagCompileUnit, Field: []dwarf.Field{
				{Attr: dwarf.AttrStmtList, Val: int64(0), Class: dwarf.ClassLinePtr},
				{Attr: dwarf.AttrCompDir, Val: "/dir", Class: dwarf.ClassString},
			}},
			{Offset: cu.Offset, Tag: dwarf.TagCompileUnit, Field: []dwarf.Field{
				{Attr: dwarf.AttrStmtList, Val: myInt64(0), Class: dwarf.ClassLinePtr},
				{Attr: dwarf.AttrCompDir, Val: "/dir", Class: dwarf.ClassString},
			}},
			{Offset: cu.Offset, Tag: dwarf.TagCompileUnit, Field: []dwarf.Field{
				{Attr: dwarf.AttrStmtList, Val: int64(100), Class: dwarf.ClassLinePtr},
				{Attr: dwarf.AttrCompDir, Val: myString("/dir"), Class: dwarf.ClassString},
			}},
			{Offset: cu.Offset, Tag: dwarf.TagCompileUnit, Field: []dwarf.Field{
				{Attr: dwarf.AttrStmtList, Val: myInt64(100), Class: dwarf.ClassLinePtr},
			}},
		}
		for i, e := range entries {
			lr, err := d.LineReader(e)
			out("program entry %d: %v %v", i, lr != nil, err)
		}
	})
}
