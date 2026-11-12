//go:build go1.27

// Native boundary of database/sql, Go 1.27 part: a driver.RowsColumnScanner
// of the program, and sql.ConvertAssign with values of the program
// (docs/BOUNDARY.md).
package main

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"strings"
	"time"
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

// The calls of database/sql to the driver, in order.
var calls []string

func call(format string, args ...any) { calls = append(calls, fmt.Sprintf(format, args...)) }

func takeCalls() string {
	s := strings.Join(calls, " ")
	calls = nil
	return s
}

// ---- the driver ----

type connector struct{ rows func() driver.Rows }

func (k connector) Connect(ctx context.Context) (driver.Conn, error) { return conn{k.rows}, nil }
func (k connector) Driver() driver.Driver                            { return nil }

type conn struct{ rows func() driver.Rows }

func (c conn) Prepare(q string) (driver.Stmt, error) { return stmt{c.rows}, nil }
func (c conn) Close() error                          { return nil }
func (c conn) Begin() (driver.Tx, error)             { return nil, fmt.Errorf("no tx") }

type stmt struct{ rows func() driver.Rows }

func (s stmt) Close() error                                    { return nil }
func (s stmt) NumInput() int                                   { return -1 }
func (s stmt) Exec(args []driver.Value) (driver.Result, error) { return driver.RowsAffected(0), nil }
func (s stmt) Query(args []driver.Value) (driver.Rows, error)  { return s.rows(), nil }

// bareRows has only the methods of driver.Rows.
type bareRows struct {
	cols []string
	data [][]driver.Value
	i    int
}

func (r *bareRows) Columns() []string { return r.cols }
func (r *bareRows) Close() error      { return nil }
func (r *bareRows) Next(dest []driver.Value) error {
	call("Next")
	if r.i >= len(r.data) {
		return io.EOF
	}
	copy(dest, r.data[r.i])
	r.i++
	return nil
}

// csRows is a driver.RowsColumnScanner: it converts with sql.ConvertAssign.
type csRows struct{ *bareRows }

func (r csRows) NextRow() error {
	call("NextRow")
	if r.i >= len(r.data) {
		return io.EOF
	}
	r.i++
	return nil
}

func (r csRows) ScanColumn(scanCtx driver.ScanContext, i int, dest any) error {
	call("ScanColumn(%d,%T)", i, dest)
	return sql.ConvertAssign(scanCtx, dest, r.data[r.i-1][i])
}

// gcsRows is a generic driver.RowsColumnScanner that sets a *T itself.
type gcsRows[T any] struct {
	*bareRows
	v T
}

func (r gcsRows[T]) NextRow() error {
	call("gNextRow")
	if r.i >= len(r.data) {
		return io.EOF
	}
	r.i++
	return nil
}

func (r gcsRows[T]) ScanColumn(scanCtx driver.ScanContext, i int, dest any) error {
	call("gScanColumn(%d,%T)", i, dest)
	if p, ok := dest.(*T); ok {
		*p = r.v
		return nil
	}
	return sql.ConvertAssign(scanCtx, dest, r.data[r.i-1][i])
}

// ---- values of the program ----

type (
	myInt   int
	myStr   string
	myBytes []byte
	myBool  bool
	myFloat float64
)

// scn is a Scanner with a pointer receiver.
type scn struct{ got string }

func (s *scn) Scan(src any) error { s.got = fmt.Sprintf("scn<%T %v>", src, src); return nil }

// gscn is a generic Scanner.
type gscn[T any] struct {
	got T
	src string
}

func (g *gscn[T]) Scan(src any) error {
	g.src = fmt.Sprintf("%T", src)
	if v, ok := src.(T); ok {
		g.got = v
		return nil
	}
	return fmt.Errorf("gscn: %T is not %T", src, g.got)
}

// vscn is a Scanner with a value receiver.
type vscn struct{ tag string }

func (v vscn) Scan(src any) error { call("vscn(%s).Scan(%T %v)", v.tag, src, src); return nil }

// dec is a decimal: Decompose (value receiver) and Compose (pointer
// receiver).
type dec struct {
	neg  bool
	coef []byte
	exp  int32
}

func (d dec) Decompose(buf []byte) (byte, bool, []byte, int32) { return 0, d.neg, d.coef, d.exp }
func (d *dec) Compose(form byte, neg bool, coef []byte, exp int32) error {
	d.neg, d.coef, d.exp = neg, append([]byte(nil), coef...), exp
	return nil
}

func dst[T any]() (any, func() string) {
	p := new(T)
	return p, func() string { return fmt.Sprintf("%T(%v)", *p, *p) }
}

func ptrDest() (any, func() string) {
	p := new(*int)
	return p, func() string {
		if *p == nil {
			return "*int(nil)"
		}
		return fmt.Sprintf("*int(&%d)", **p)
	}
}

func rowsDest() (any, func() string) {
	p := new(sql.Rows)
	return p, func() string {
		var got []string
		for p.Next() {
			var s string
			err := p.Scan(&s)
			got = append(got, fmt.Sprintf("%q %v", s, err))
		}
		return fmt.Sprintf("child %v err %v close %v", got, p.Err(), p.Close())
	}
}

type scase struct {
	src  func() driver.Value
	dest func() (any, func() string)
}

func src(v driver.Value) func() driver.Value { return func() driver.Value { return v } }

func cursor() driver.Value {
	return &bareRows{cols: []string{"c"}, data: [][]driver.Value{{"child1"}, {myStr("child2")}}}
}

func main() {
	cases := []scase{
		{src(int64(7)), dst[string]},
		{src(int64(7)), dst[sql.RawBytes]},
		{src(int64(7)), dst[myInt]},
		{src(int64(7)), ptrDest},
		{src(int64(7)), dst[scn]},
		{src(int64(7)), dst[gscn[int64]]},
		{src(int64(7)), func() (any, func() string) { return vscn{"v"}, func() string { return takeCalls() } }},
		{src(int64(7)), func() (any, func() string) { return myInt(0), func() string { return "-" } }},
		{src(myInt(7)), dst[string]},
		{src(myInt(7)), dst[[]byte]},
		{src(myInt(7)), dst[int]},
		{src(myInt(7)), dst[bool]},
		{src(myInt(1)), dst[bool]},
		{src(myStr("42")), dst[string]},
		{src(myStr("42")), dst[[]byte]},
		{src(myStr("42")), dst[int]},
		{src(myStr("42")), dst[myStr]},
		{src(myBytes("xy")), dst[[]byte]},
		{src(myBytes("xy")), dst[string]},
		{src(myBool(true)), dst[bool]},
		{src(myBool(true)), dst[string]},
		{src(myFloat(1.5)), dst[float32]},
		{src(myFloat(1.5)), dst[[]byte]},
		{src("s"), dst[myStr]},
		{src("12"), dst[myInt]},
		{src(nil), dst[myInt]},
		{src(nil), ptrDest},
		{src(dec{true, []byte{1, 2}, -2}), dst[dec]},
		{src(dec{true, []byte{1, 2}, -2}), dst[string]},
		{src(time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)), dst[myStr]},
		{cursor, rowsDest},
		{cursor, dst[any]},
	}

	// row: database/sql ConvertAssign assert x.(T), T unnamed
	// row: database/sql ConvertAssign assert x.(database/sql.Scanner)
	// row: database/sql ConvertAssign assert x.(database/sql.decimalCompose)
	// row: database/sql ConvertAssign assert x.(database/sql.decimalDecompose)
	// row: database/sql ConvertAssign assert x.(database/sql/driver.Rows)
	// row: database/sql ConvertAssign panic x.(T) without ok, T unnamed: *runtime.TypeAssertionError
	// row: database/sql ConvertAssign reflect kind
	// row: database/sql ConvertAssign reflect type name
	// row: database/sql ConvertAssign reflect walk
	probe("ConvertAssign", func() {
		for i, c := range cases {
			s := c.src()
			dest, show := c.dest()
			// A zero ScanContext with a cursor panics in Go 1.27: each
			// case recovers on its own.
			func() {
				defer func() {
					if r := recover(); r != nil {
						out("%d %T <- %T: panic %T: %v", i, dest, s, r, r)
					}
				}()
				err := sql.ConvertAssign(driver.ScanContext{}, dest, s)
				out("%d %T <- %T: %v => %s", i, dest, s, err, show())
			}()
		}
		takeCalls()
	})

	// row: database/sql (*Row).Scan,(*Rows).Next,(*Rows).Scan assert x.(database/sql/driver.RowsColumnScanner)
	probe("RowsColumnScanner", func() {
		kinds := []func(b *bareRows) driver.Rows{
			func(b *bareRows) driver.Rows { return b },
			func(b *bareRows) driver.Rows { return csRows{b} },
			func(b *bareRows) driver.Rows { return gcsRows[myInt]{b, 99} },
		}
		for k, mk := range kinds {
			for i, c := range cases {
				// Each case has its own DB and recovers its panic.
				func() {
					rows := func() driver.Rows {
						return mk(&bareRows{cols: []string{"v"}, data: [][]driver.Value{{c.src()}}})
					}
					db := sql.OpenDB(connector{rows})
					defer db.Close()
					defer func() {
						if r := recover(); r != nil {
							out("%d/%d: panic: %T: %v | %s", k, i, r, r, takeCalls())
						}
					}()
					s := c.src()
					dest, show := c.dest()
					rs, err := db.Query("q")
					if err != nil {
						out("%d/%d: query %v", k, i, err)
						return
					}
					next := rs.Next()
					err = rs.Scan(dest)
					out("%d/%d Rows.Scan %T <- %T: next %v %v => %s | %s", k, i, dest, s, next, err, show(), takeCalls())
					rs.Close()
					dest, show = c.dest()
					err = db.QueryRow("q").Scan(dest)
					out("%d/%d Row.Scan %T <- %T: %v => %s | %s", k, i, dest, s, err, show(), takeCalls())
				}()
			}
		}
	})
}
