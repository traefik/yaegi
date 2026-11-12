// Native boundary of database/sql and database/sql/driver: a driver, Valuers,
// Scanners and decimals of the program given to database/sql
// (docs/BOUNDARY.md).
package main

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
	"sync"
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

// The calls of database/sql to the driver, in order. A goroutine of
// database/sql may call the driver, so the list has a lock.
var (
	mu    sync.Mutex
	calls []string
)

func call(format string, args ...any) {
	mu.Lock()
	calls = append(calls, fmt.Sprintf(format, args...))
	mu.Unlock()
}

// takeCalls gives the calls since the last take, and clears them.
func takeCalls() string {
	mu.Lock()
	defer mu.Unlock()
	s := strings.Join(calls, " ")
	calls = nil
	return s
}

// takeSorted gives the calls since the last take, sorted (for calls made
// by a goroutine of database/sql).
func takeSorted() string {
	mu.Lock()
	defer mu.Unlock()
	sort.Strings(calls)
	s := strings.Join(calls, " ")
	calls = nil
	return s
}

// done prints one line: the step, its result and the calls of the driver.
func done(step, result string) { out("%s: %s | %s", step, result, takeCalls()) }

func vals(vs []driver.Value) string {
	var p []string
	for _, v := range vs {
		p = append(p, fmt.Sprintf("%T(%v)", v, v))
	}
	return "[" + strings.Join(p, " ") + "]"
}

func nvals(vs []driver.NamedValue) string {
	var p []string
	for _, v := range vs {
		p = append(p, fmt.Sprintf("%s#%d=%T(%v)", v.Name, v.Ordinal, v.Value, v.Value))
	}
	return "[" + strings.Join(p, " ") + "]"
}

// ---- the driver ----

// cfg selects the types that the driver gives to database/sql.
type cfg struct {
	conn  func(b *bareConn) driver.Conn
	stmt  func(s *bareStmt) driver.Stmt
	rows  func() driver.Rows
	nIn   int
	nConn int
	// flaky: the second IsValid says false, the second ResetSession
	// says driver.ErrBadConn.
	flaky          bool
	nValid, nReset int
}

func newCfg() *cfg {
	return &cfg{
		conn: func(b *bareConn) driver.Conn { return b },
		stmt: func(s *bareStmt) driver.Stmt { return s },
		rows: oneRow,
		nIn:  -1,
	}
}

func oneRow() driver.Rows {
	return &bareRows{cols: []string{"n"}, data: [][]driver.Value{{int64(1)}}}
}

// connector is a driver.Connector of the program.
type connector struct{ c *cfg }

func (k connector) Connect(ctx context.Context) (driver.Conn, error) {
	mu.Lock()
	k.c.nConn++
	id := k.c.nConn
	mu.Unlock()
	call("Connect%d", id)
	return k.c.conn(&bareConn{c: k.c, id: id}), nil
}

func (k connector) Driver() driver.Driver { return drv{k.c} }

// closingConnector is a connector with Close (io.Closer).
type closingConnector struct{ connector }

func (k closingConnector) Close() error {
	call("Connector.Close")
	return errors.New("connector close error")
}

// gcloser is a generic connector with Close.
type gcloser[T any] struct {
	connector
	tag T
}

func (k *gcloser[T]) Close() error { call("gcloser[%v].Close", k.tag); return nil }

// drv is a driver.Driver of the program.
type drv struct{ c *cfg }

func (d drv) Open(name string) (driver.Conn, error) {
	call("drv.Open(%s)", name)
	return connector{d.c}.Connect(context.Background())
}

// gdrv is a generic driver with OpenConnector (driver.DriverContext).
type gdrv[T any] struct {
	c   *cfg
	tag T
}

func (d gdrv[T]) Open(name string) (driver.Conn, error) {
	call("gdrv.Open(%s)", name)
	return connector{d.c}.Connect(context.Background())
}

func (d gdrv[T]) OpenConnector(name string) (driver.Connector, error) {
	call("gdrv[%v].OpenConnector(%s)", d.tag, name)
	return connector{d.c}, nil
}

// bareConn has only the methods of driver.Conn.
type bareConn struct {
	c  *cfg
	id int
}

func (b *bareConn) Prepare(q string) (driver.Stmt, error) {
	call("c%d.Prepare(%s)", b.id, q)
	return b.c.stmt(&bareStmt{c: b.c, q: q}), nil
}
func (b *bareConn) Close() error { call("c%d.Close", b.id); return nil }
func (b *bareConn) Begin() (driver.Tx, error) {
	call("c%d.Begin", b.id)
	return tx{b.id}, nil
}

// pcConn has PrepareContext.
type pcConn struct{ *bareConn }

func (c pcConn) PrepareContext(ctx context.Context, q string) (driver.Stmt, error) {
	call("c%d.PrepareContext(%s)", c.id, q)
	return c.c.stmt(&bareStmt{c: c.c, q: q}), nil
}

// btConn has BeginTx.
type btConn struct{ *bareConn }

func (c *btConn) BeginTx(ctx context.Context, o driver.TxOptions) (driver.Tx, error) {
	call("c%d.BeginTx(iso=%d,ro=%v)", c.id, o.Isolation, o.ReadOnly)
	return tx{c.id}, nil
}

// pingConn has Ping.
type pingConn struct{ *bareConn }

func (c pingConn) Ping(ctx context.Context) error {
	call("c%d.Ping", c.id)
	return nil
}

// gpingConn is a generic conn with Ping that fails.
type gpingConn[T any] struct {
	*bareConn
	tag T
}

func (c gpingConn[T]) Ping(ctx context.Context) error {
	call("c%d.gPing", c.id)
	return fmt.Errorf("ping %v failed", c.tag)
}

// execCtxConn has ExecContext and QueryContext.
type execCtxConn struct{ *bareConn }

func (c execCtxConn) ExecContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Result, error) {
	call("c%d.ExecContext(%s)%s", c.id, q, nvals(args))
	if q == "skip" {
		return nil, driver.ErrSkip
	}
	return driver.RowsAffected(len(args)), nil
}

func (c execCtxConn) QueryContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Rows, error) {
	call("c%d.QueryContext(%s)%s", c.id, q, nvals(args))
	if q == "skip" {
		return nil, driver.ErrSkip
	}
	return c.c.rows(), nil
}

// execConn has the old Exec and Query (driver.Execer, driver.Queryer).
type execConn struct{ *bareConn }

func (c execConn) Exec(q string, args []driver.Value) (driver.Result, error) {
	call("c%d.Exec(%s)%s", c.id, q, vals(args))
	if q == "skip" {
		return nil, driver.ErrSkip
	}
	return driver.RowsAffected(len(args)), nil
}

func (c execConn) Query(q string, args []driver.Value) (driver.Rows, error) {
	call("c%d.Query(%s)%s", c.id, q, vals(args))
	if q == "skip" {
		return nil, driver.ErrSkip
	}
	return c.c.rows(), nil
}

// nvcConn has CheckNamedValue.
type nvcConn struct{ *bareConn }

func (c nvcConn) CheckNamedValue(nv *driver.NamedValue) error { return check("conn", nv) }

// check is the CheckNamedValue of the program: it changes myInt, skips,
// removes or refuses some strings, and keeps the other values as they are.
func check(who string, nv *driver.NamedValue) error {
	call("%s.Check(%T)", who, nv.Value)
	switch v := nv.Value.(type) {
	case myInt:
		nv.Value = int64(v) * 10
		return nil
	case string:
		switch v {
		case "skip":
			return driver.ErrSkip
		case "remove":
			return driver.ErrRemoveArgument
		case "bad":
			return errors.New("bad arg")
		}
	}
	return nil
}

// resetConn has ResetSession and IsValid.
type resetConn struct{ *bareConn }

func (c resetConn) ResetSession(ctx context.Context) error {
	mu.Lock()
	c.c.nReset++
	n := c.c.nReset
	mu.Unlock()
	call("c%d.ResetSession", c.id)
	if c.c.flaky && n == 2 {
		return driver.ErrBadConn
	}
	return nil
}

func (c resetConn) IsValid() bool {
	mu.Lock()
	c.c.nValid++
	n := c.c.nValid
	mu.Unlock()
	call("c%d.IsValid", c.id)
	return !(c.c.flaky && n == 2)
}

// validConn has IsValid only.
type validConn struct{ *bareConn }

func (c validConn) IsValid() bool { call("c%d.IsValid", c.id); return true }

type tx struct{ id int }

// rolled gets a value when a Rollback is done, if it is not nil.
var rolled chan struct{}

func (t tx) Commit() error { call("c%d.Commit", t.id); return nil }
func (t tx) Rollback() error {
	call("c%d.Rollback", t.id)
	if rolled != nil {
		rolled <- struct{}{}
	}
	return nil
}

// bareStmt has only the methods of driver.Stmt.
type bareStmt struct {
	c *cfg
	q string
}

func (s *bareStmt) Close() error  { call("s.Close"); return nil }
func (s *bareStmt) NumInput() int { return s.c.nIn }
func (s *bareStmt) Exec(args []driver.Value) (driver.Result, error) {
	call("s.Exec%s", vals(args))
	return driver.RowsAffected(len(args)), nil
}

func (s *bareStmt) Query(args []driver.Value) (driver.Rows, error) {
	call("s.Query%s", vals(args))
	return s.c.rows(), nil
}

// ctxStmt has ExecContext and QueryContext.
type ctxStmt struct{ *bareStmt }

func (s ctxStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	call("s.ExecContext%s", nvals(args))
	return driver.RowsAffected(len(args)), nil
}

func (s ctxStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	call("s.QueryContext%s", nvals(args))
	return s.c.rows(), nil
}

// nvcStmt has CheckNamedValue.
type nvcStmt struct{ *bareStmt }

func (s nvcStmt) CheckNamedValue(nv *driver.NamedValue) error { return check("stmt", nv) }

// ccStmt has ColumnConverter.
type ccStmt struct{ *bareStmt }

func (s ccStmt) ColumnConverter(i int) driver.ValueConverter {
	call("cc(%d)", i)
	return colConv{i}
}

// nvcccStmt has CheckNamedValue and ColumnConverter.
type nvcccStmt struct{ *bareStmt }

func (s nvcccStmt) CheckNamedValue(nv *driver.NamedValue) error { return check("stmt", nv) }
func (s nvcccStmt) ColumnConverter(i int) driver.ValueConverter {
	call("cc(%d)", i)
	return colConv{i}
}

// colConv is a driver.ValueConverter of the program.
type colConv struct{ i int }

func (c colConv) ConvertValue(v any) (driver.Value, error) {
	call("conv%d(%T)", c.i, v)
	switch v := v.(type) {
	case myInt:
		return myInt(v + 1), nil // not a driver.Value: refused by database/sql
	case string:
		return "cc:" + v, nil
	}
	return driver.DefaultParameterConverter.ConvertValue(v)
}

// bareRows has only the methods of driver.Rows.
type bareRows struct {
	cols []string
	data [][]driver.Value
	i    int
}

func (r *bareRows) Columns() []string { return r.cols }
func (r *bareRows) Close() error      { return nil }
func (r *bareRows) Next(dest []driver.Value) error {
	if r.i >= len(r.data) {
		return io.EOF
	}
	copy(dest, r.data[r.i])
	r.i++
	return nil
}

// ctRows has all the RowsColumnType methods.
type ctRows struct{ *bareRows }

func (r ctRows) ColumnTypeScanType(i int) reflect.Type {
	return []reflect.Type{reflect.TypeOf(myInt(0)), reflect.TypeOf(gval[string]{}), reflect.TypeOf(&scn{})}[i]
}
func (r ctRows) ColumnTypeDatabaseTypeName(i int) string { return fmt.Sprintf("DBT%d", i) }
func (r ctRows) ColumnTypeLength(i int) (int64, bool)    { return int64(10 * i), i == 0 }
func (r ctRows) ColumnTypeNullable(i int) (bool, bool)   { return i == 1, i != 2 }
func (r ctRows) ColumnTypePrecisionScale(i int) (int64, int64, bool) {
	return int64(5 + i), 2, i == 1
}

// gctRows is a generic rows with ColumnTypeScanType only.
type gctRows[T any] struct{ *bareRows }

func (r gctRows[T]) ColumnTypeScanType(i int) reflect.Type { return reflect.TypeOf((*T)(nil)).Elem() }

// nrsRows has more than one result set (driver.RowsNextResultSet).
type nrsRows struct {
	cols []string
	sets [][][]driver.Value
	set  int
	i    int
}

func (r *nrsRows) Columns() []string { return r.cols }
func (r *nrsRows) Close() error      { call("nrs.Close"); return nil }
func (r *nrsRows) Next(dest []driver.Value) error {
	if r.set >= len(r.sets) || r.i >= len(r.sets[r.set]) {
		return io.EOF
	}
	copy(dest, r.sets[r.set][r.i])
	r.i++
	return nil
}

func (r *nrsRows) HasNextResultSet() bool {
	call("Has(%d)", r.set)
	return r.set+1 < len(r.sets)
}

func (r *nrsRows) NextResultSet() error {
	call("NextResultSet(%d)", r.set)
	r.set++
	r.i = 0
	if r.set >= len(r.sets) {
		return io.EOF
	}
	return nil
}

// ---- values of the program ----

type (
	myInt   int
	myStr   string
	myBytes []byte
	myBool  bool
	myFloat float64
)

// vval is a Valuer with a value receiver.
type vval struct{ s string }

func (v vval) Value() (driver.Value, error) { return "vval:" + v.s, nil }

// pval is a Valuer with a pointer receiver.
type pval struct{ n int64 }

func (p *pval) Value() (driver.Value, error) {
	if p == nil {
		return "nil-pval", nil
	}
	return p.n * 100, nil
}

// gval is a generic Valuer.
type gval[T any] struct{ v T }

func (g gval[T]) Value() (driver.Value, error) { return g.v, nil }

// badVal gives a value that is not a driver.Value.
type badVal struct{}

func (badVal) Value() (driver.Value, error) { return myInt(1), nil }

// errVal fails.
type errVal struct{}

func (errVal) Value() (driver.Value, error) { return nil, errors.New("errVal failed") }

type plain struct{ a int }

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

// ---- helpers ----

func open(c *cfg) *sql.DB {
	db := sql.OpenDB(connector{c})
	db.SetMaxOpenConns(1)
	return db
}

func res(step string, r sql.Result, err error) {
	if err != nil {
		done(step, "error "+err.Error())
		return
	}
	n, err := r.RowsAffected()
	done(step, fmt.Sprintf("affected %d %v", n, err))
}

func rowsRes(step string, rs *sql.Rows, err error) {
	if err != nil {
		done(step, "error "+err.Error())
		return
	}
	var got []string
	for rs.Next() {
		var v any
		err := rs.Scan(&v)
		got = append(got, fmt.Sprintf("%T(%v) %v", v, v, err))
	}
	done(step, fmt.Sprintf("rows %v err %v close %v", got, rs.Err(), rs.Close()))
}

func rowRes(step string, r *sql.Row) {
	var v any
	err := r.Scan(&v)
	done(step, fmt.Sprintf("row %T(%v) %v", v, v, err))
}

// every runs q with args through every entry point that converts arguments:
// Exec, Query and QueryRow of DB, Conn, Stmt and Tx.
func every(db *sql.DB, q string, args ...any) {
	ctx := context.Background()
	r, err := db.Exec(q, args...)
	res("DB.Exec", r, err)
	r, err = db.ExecContext(ctx, q, args...)
	res("DB.ExecContext", r, err)
	rs, err := db.Query(q, args...)
	rowsRes("DB.Query", rs, err)
	rs, err = db.QueryContext(ctx, q, args...)
	rowsRes("DB.QueryContext", rs, err)
	rowRes("DB.QueryRow", db.QueryRow(q, args...))
	rowRes("DB.QueryRowContext", db.QueryRowContext(ctx, q, args...))

	conn, err := db.Conn(ctx)
	if err != nil {
		done("DB.Conn", "error "+err.Error())
		return
	}
	r, err = conn.ExecContext(ctx, q, args...)
	res("Conn.ExecContext", r, err)
	rs, err = conn.QueryContext(ctx, q, args...)
	rowsRes("Conn.QueryContext", rs, err)
	rowRes("Conn.QueryRowContext", conn.QueryRowContext(ctx, q, args...))
	done("Conn.Close", fmt.Sprint(conn.Close()))

	stmt, err := db.Prepare(q)
	if err != nil {
		done("DB.Prepare", "error "+err.Error())
		return
	}
	done("DB.Prepare", "ok")
	r, err = stmt.Exec(args...)
	res("Stmt.Exec", r, err)
	r, err = stmt.ExecContext(ctx, args...)
	res("Stmt.ExecContext", r, err)
	rs, err = stmt.Query(args...)
	rowsRes("Stmt.Query", rs, err)
	rs, err = stmt.QueryContext(ctx, args...)
	rowsRes("Stmt.QueryContext", rs, err)
	rowRes("Stmt.QueryRow", stmt.QueryRow(args...))
	rowRes("Stmt.QueryRowContext", stmt.QueryRowContext(ctx, args...))
	done("Stmt.Close", fmt.Sprint(stmt.Close()))

	t, err := db.Begin()
	if err != nil {
		done("DB.Begin", "error "+err.Error())
		return
	}
	done("DB.Begin", "ok")
	r, err = t.Exec(q, args...)
	res("Tx.Exec", r, err)
	r, err = t.ExecContext(ctx, q, args...)
	res("Tx.ExecContext", r, err)
	rs, err = t.Query(q, args...)
	rowsRes("Tx.Query", rs, err)
	rs, err = t.QueryContext(ctx, q, args...)
	rowsRes("Tx.QueryContext", rs, err)
	rowRes("Tx.QueryRow", t.QueryRow(q, args...))
	rowRes("Tx.QueryRowContext", t.QueryRowContext(ctx, q, args...))
	done("Tx.Commit", fmt.Sprint(t.Commit()))
}

// dst makes a new destination of type *T and a func that shows its value.
func dst[T any]() (any, func() string) {
	p := new(T)
	return p, func() string { return fmt.Sprintf("%T(%v)", *p, *p) }
}

// scase is one Scan: a source value given by the driver, a destination.
type scase struct {
	src  func() driver.Value
	dest func() (any, func() string)
}

// ptrDest is a **int destination: its show prints the int, not the pointer.
func ptrDest() (any, func() string) {
	p := new(*int)
	return p, func() string {
		if *p == nil {
			return "*int(nil)"
		}
		return fmt.Sprintf("*int(&%d)", **p)
	}
}

func src(v driver.Value) func() driver.Value { return func() driver.Value { return v } }

func cursor() driver.Value {
	return &bareRows{cols: []string{"c"}, data: [][]driver.Value{{"child1"}, {myStr("child2")}}}
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

// scanAll scans each case with Rows.Scan and with Row.Scan. Each case has
// its own DB and recovers its panic: a panic does not hide the next cases.
func scanAll(cases []scase) {
	for i, sc := range cases {
		scanOne(i, sc)
		takeCalls()
	}
}

func scanOne(i int, sc scase) {
	c := newCfg()
	c.rows = func() driver.Rows {
		return &bareRows{cols: []string{"v"}, data: [][]driver.Value{{sc.src()}}}
	}
	db := sql.OpenDB(connector{c})
	defer db.Close()
	defer func() {
		if r := recover(); r != nil {
			out("%d: panic: %T: %v", i, r, r)
		}
	}()
	s := sc.src()
	dest, show := sc.dest()
	rs, err := db.Query("q")
	if err != nil {
		out("%d: query %v", i, err)
		return
	}
	rs.Next()
	err = rs.Scan(dest)
	out("%d Rows.Scan %T <- %T: %v => %s", i, dest, s, err, show())
	rs.Close()
	dest, show = sc.dest()
	err = db.QueryRow("q").Scan(dest)
	out("%d Row.Scan %T <- %T: %v => %s", i, dest, s, err, show())
}

func main() {
	ctx := context.Background()

	// row: database/sql (*Conn).PingContext,(*DB).Ping,(*DB).PingContext assert x.(database/sql/driver.Pinger)
	probe("Ping", func() {
		takeCalls() // the calls of a probe that panicked
		mk := []func(b *bareConn) driver.Conn{
			func(b *bareConn) driver.Conn { return b },
			func(b *bareConn) driver.Conn { return pingConn{b} },
			func(b *bareConn) driver.Conn { return gpingConn[string]{b, "g"} },
		}
		for i, m := range mk {
			c := newCfg()
			c.conn = m
			db := open(c)
			done(fmt.Sprintf("%d DB.Ping", i), fmt.Sprint(db.Ping()))
			done(fmt.Sprintf("%d DB.PingContext", i), fmt.Sprint(db.PingContext(ctx)))
			conn, err := db.Conn(ctx)
			if err == nil {
				done(fmt.Sprintf("%d Conn.PingContext", i), fmt.Sprint(conn.PingContext(ctx)))
				conn.Close()
			}
			done(fmt.Sprintf("%d DB.Close", i), fmt.Sprint(db.Close()))
		}
	})

	// row: database/sql (*Conn).BeginTx,(*DB).Begin,(*DB).BeginTx assert x.(database/sql/driver.ConnBeginTx)
	probe("BeginTx", func() {
		takeCalls() // the calls of a probe that panicked
		mk := []func(b *bareConn) driver.Conn{
			func(b *bareConn) driver.Conn { return b },
			func(b *bareConn) driver.Conn { return &btConn{b} },
		}
		for i, m := range mk {
			c := newCfg()
			c.conn = m
			db := open(c)
			step := func(name string, t *sql.Tx, err error) {
				if err != nil {
					done(fmt.Sprintf("%d %s", i, name), "error "+err.Error())
					return
				}
				done(fmt.Sprintf("%d %s", i, name), "commit "+fmt.Sprint(t.Commit()))
			}
			t, err := db.Begin()
			step("DB.Begin", t, err)
			t, err = db.BeginTx(ctx, nil)
			step("DB.BeginTx nil", t, err)
			t, err = db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
			step("DB.BeginTx ro", t, err)
			t, err = db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
			step("DB.BeginTx serializable", t, err)
			conn, err := db.Conn(ctx)
			if err == nil {
				t, err = conn.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
				step("Conn.BeginTx", t, err)
				t, err = conn.BeginTx(ctx, nil)
				if err == nil {
					done(fmt.Sprintf("%d Conn.BeginTx rollback", i), fmt.Sprint(t.Rollback()))
				}
				conn.Close()
			}
			done(fmt.Sprintf("%d DB.Close", i), fmt.Sprint(db.Close()))
		}
	})

	// row: database/sql (*Conn).BeginTx,(*DB).Begin,(*DB).BeginTx,(*DB).Conn,(*DB).Exec,(*DB).ExecContext,(*DB).Ping,(*DB).PingContext,(*DB).Prepare,(*DB).PrepareContext,(*DB).Query,(*DB).QueryContext,(*DB).QueryRow,(*DB).QueryRowContext,(*Stmt).Exec,(*Stmt).ExecContext,(*Stmt).Query,(*Stmt).QueryContext,(*Stmt).QueryRow,(*Stmt).QueryRowContext assert x.(database/sql/driver.SessionResetter)
	// row: database/sql (*Conn).BeginTx,(*Conn).Close,(*Conn).ExecContext,(*Conn).PingContext,(*Conn).PrepareContext,(*Conn).QueryContext,(*Conn).QueryRowContext,(*Conn).Raw,(*DB).Begin,(*DB).BeginTx,(*DB).Conn,(*DB).Exec,(*DB).ExecContext,(*DB).Ping,(*DB).PingContext,(*DB).Prepare,(*DB).PrepareContext,(*DB).Query,(*DB).QueryContext,(*DB).QueryRow,(*DB).QueryRowContext,(*Stmt).Exec,(*Stmt).ExecContext,(*Stmt).Query,(*Stmt).QueryContext,(*Stmt).QueryRow,(*Stmt).QueryRowContext assert x.(database/sql/driver.Validator)
	probe("Session", func() {
		takeCalls() // the calls of a probe that panicked
		mk := []func(b *bareConn) driver.Conn{
			func(b *bareConn) driver.Conn { return b },
			func(b *bareConn) driver.Conn { return resetConn{b} },
			func(b *bareConn) driver.Conn { return validConn{b} },
			func(b *bareConn) driver.Conn { return resetConn{b} }, // flaky
		}
		for i, m := range mk {
			out("conn %d", i)
			c := newCfg()
			c.conn = m
			c.flaky = i == 3
			db := open(c)
			done("DB.Ping", fmt.Sprint(db.Ping()))
			done("DB.PingContext", fmt.Sprint(db.PingContext(ctx)))
			every(db, "q", 1)
			st, err := db.Prepare("p")
			if err == nil {
				st.Close()
			}
			done("DB.Prepare", fmt.Sprint(err))
			st, err = db.PrepareContext(ctx, "pc")
			if err == nil {
				st.Close()
			}
			done("DB.PrepareContext", fmt.Sprint(err))
			t, err := db.BeginTx(ctx, nil)
			if err == nil {
				err = t.Rollback()
			}
			done("DB.BeginTx", fmt.Sprint(err))
			conn, err := db.Conn(ctx)
			if err != nil {
				done("DB.Conn", fmt.Sprint(err))
			} else {
				done("DB.Conn", "ok")
				t, err := conn.BeginTx(ctx, nil)
				if err == nil {
					err = t.Commit()
				}
				done("Conn.BeginTx", fmt.Sprint(err))
				done("Conn.PingContext", fmt.Sprint(conn.PingContext(ctx)))
				st, err := conn.PrepareContext(ctx, "cp")
				if err == nil {
					st.Close()
				}
				done("Conn.PrepareContext", fmt.Sprint(err))
				err = conn.Raw(func(dc any) error {
					_, v := dc.(driver.Validator)
					_, r := dc.(driver.SessionResetter)
					out("Raw: %T validator %v resetter %v", dc, v, r)
					return nil
				})
				done("Conn.Raw", fmt.Sprint(err))
				done("Conn.Close", fmt.Sprint(conn.Close()))
			}
			done("DB.Close", fmt.Sprint(db.Close()))
		}
	})

	// row: database/sql (*Conn).BeginTx,(*DB).Begin,(*DB).BeginTx assert x.(database/sql/driver.SessionResetter)
	// row: database/sql (*Conn).BeginTx,(*DB).Begin,(*DB).BeginTx assert x.(database/sql/driver.Validator)
	probe("BeginTx/cancel", func() {
		takeCalls() // the calls of a probe that panicked
		// A canceled transaction keeps its conn only when the conn has
		// ResetSession and IsValid.
		mk := []func(b *bareConn) driver.Conn{
			func(b *bareConn) driver.Conn { return b },
			func(b *bareConn) driver.Conn { return resetConn{b} },
			func(b *bareConn) driver.Conn { return validConn{b} },
		}
		for i, m := range mk {
			c := newCfg()
			c.conn = m
			db := open(c)
			rolled = make(chan struct{}, 1)
			cctx, cancel := context.WithCancel(ctx)
			_, err := db.BeginTx(cctx, nil)
			if err != nil {
				out("%d: %v", i, err)
				cancel()
				continue
			}
			cancel()
			<-rolled
			rolled = nil
			_, err = db.Exec("after")
			out("%d: exec %v close %v | %s", i, err, db.Close(), takeSorted())
		}
		// Conn.BeginTx and DB.Begin on the same kinds of conns.
		for i, m := range mk {
			c := newCfg()
			c.conn = m
			db := open(c)
			t, err := db.Begin()
			if err == nil {
				err = t.Rollback()
			}
			done(fmt.Sprintf("%d DB.Begin", i), fmt.Sprint(err))
			conn, err := db.Conn(ctx)
			if err == nil {
				t, err := conn.BeginTx(ctx, nil)
				if err == nil {
					err = t.Rollback()
				}
				done(fmt.Sprintf("%d Conn.BeginTx", i), fmt.Sprint(err))
				conn.Close()
			}
			done(fmt.Sprintf("%d DB.Close", i), fmt.Sprint(db.Close()))
		}
	})

	// row: database/sql (*Conn).ExecContext,(*Conn).PrepareContext,(*Conn).QueryContext,(*Conn).QueryRowContext,(*DB).Exec,(*DB).ExecContext,(*DB).Prepare,(*DB).PrepareContext,(*DB).Query,(*DB).QueryContext,(*DB).QueryRow,(*DB).QueryRowContext,(*Stmt).Exec,(*Stmt).ExecContext,(*Stmt).Query,(*Stmt).QueryContext,(*Stmt).QueryRow,(*Stmt).QueryRowContext,(*Tx).Exec,(*Tx).ExecContext,(*Tx).Prepare,(*Tx).PrepareContext,(*Tx).Query,(*Tx).QueryContext,(*Tx).QueryRow,(*Tx).QueryRowContext,(*Tx).Stmt,(*Tx).StmtContext assert x.(database/sql/driver.ConnPrepareContext)
	probe("PrepareContext", func() {
		takeCalls() // the calls of a probe that panicked
		mk := []func(b *bareConn) driver.Conn{
			func(b *bareConn) driver.Conn { return b },
			func(b *bareConn) driver.Conn { return pcConn{b} },
		}
		for i, m := range mk {
			out("conn %d", i)
			c := newCfg()
			c.conn = m
			db := open(c)
			every(db, "q", 1)
			done("DB.Close", fmt.Sprint(db.Close()))

			// Tx.Stmt prepares the statement again on the conn of the
			// transaction: no limit of conns here.
			c = newCfg()
			c.conn = m
			db = sql.OpenDB(connector{c})
			t, err := db.Begin()
			if err != nil {
				out("Begin: %v", err)
				continue
			}
			st1, err1 := db.Prepare("s1")
			st2, err2 := db.PrepareContext(ctx, "s2")
			done("DB.Prepare", fmt.Sprint(err1, err2))
			if err1 == nil && err2 == nil {
				ts := t.Stmt(st1)
				r, err := ts.Exec(1)
				res("Tx.Stmt", r, err)
				ts = t.StmtContext(ctx, st2)
				r, err = ts.Exec(2)
				res("Tx.StmtContext", r, err)
				st1.Close()
				st2.Close()
			}
			ts, err := t.Prepare("tp")
			if err == nil {
				r, err := ts.Exec(3)
				res("Tx.Prepare", r, err)
			}
			ts, err = t.PrepareContext(ctx, "tpc")
			if err == nil {
				r, err := ts.Exec(4)
				res("Tx.PrepareContext", r, err)
			}
			done("Tx.Commit", fmt.Sprint(t.Commit()))
			conn, err := db.Conn(ctx)
			if err == nil {
				cs, err := conn.PrepareContext(ctx, "cp")
				if err == nil {
					cs.Close()
				}
				done("Conn.PrepareContext", fmt.Sprint(err))
				conn.Close()
			}
			done("DB.Close", fmt.Sprint(db.Close()))
		}
	})

	// row: database/sql (*Conn).ExecContext,(*DB).Exec,(*DB).ExecContext,(*Tx).Exec,(*Tx).ExecContext assert x.(database/sql/driver.ExecerContext)
	// row: database/sql (*Conn).ExecContext,(*DB).Exec,(*DB).ExecContext,(*Tx).Exec,(*Tx).ExecContext assert x.(database/sql/driver.Execer)
	// row: database/sql (*Conn).QueryContext,(*Conn).QueryRowContext,(*DB).Query,(*DB).QueryContext,(*DB).QueryRow,(*DB).QueryRowContext,(*Tx).Query,(*Tx).QueryContext,(*Tx).QueryRow,(*Tx).QueryRowContext assert x.(database/sql/driver.QueryerContext)
	// row: database/sql (*Conn).QueryContext,(*Conn).QueryRowContext,(*DB).Query,(*DB).QueryContext,(*DB).QueryRow,(*DB).QueryRowContext,(*Tx).Query,(*Tx).QueryContext,(*Tx).QueryRow,(*Tx).QueryRowContext assert x.(database/sql/driver.Queryer)
	probe("Execer", func() {
		takeCalls() // the calls of a probe that panicked
		mk := []func(b *bareConn) driver.Conn{
			func(b *bareConn) driver.Conn { return b },
			func(b *bareConn) driver.Conn { return execCtxConn{b} },
			func(b *bareConn) driver.Conn { return execConn{b} },
		}
		for i, m := range mk {
			for _, q := range []string{"q", "skip"} {
				out("conn %d %s", i, q)
				c := newCfg()
				c.conn = m
				db := open(c)
				every(db, q, 1, "a")
				r, err := db.Exec(q, sql.Named("n", 2))
				res("DB.Exec named", r, err)
				done("DB.Close", fmt.Sprint(db.Close()))
			}
		}
	})

	// row: database/sql (*Conn).ExecContext,(*DB).Exec,(*DB).ExecContext,(*Stmt).Exec,(*Stmt).ExecContext,(*Tx).Exec,(*Tx).ExecContext assert x.(database/sql/driver.StmtExecContext)
	// row: database/sql (*Conn).QueryContext,(*Conn).QueryRowContext,(*DB).Query,(*DB).QueryContext,(*DB).QueryRow,(*DB).QueryRowContext,(*Stmt).Query,(*Stmt).QueryContext,(*Stmt).QueryRow,(*Stmt).QueryRowContext,(*Tx).Query,(*Tx).QueryContext,(*Tx).QueryRow,(*Tx).QueryRowContext assert x.(database/sql/driver.StmtQueryContext)
	probe("StmtContext", func() {
		takeCalls() // the calls of a probe that panicked
		for i, st := range []func(s *bareStmt) driver.Stmt{
			func(s *bareStmt) driver.Stmt { return s },
			func(s *bareStmt) driver.Stmt { return ctxStmt{s} },
		} {
			out("stmt %d", i)
			c := newCfg()
			c.stmt = st
			db := open(c)
			every(db, "q", 1, sql.Named("x", "y"))
			done("DB.Close", fmt.Sprint(db.Close()))
		}
	})

	// row: database/sql (*Conn).ExecContext,(*Conn).QueryContext,(*Conn).QueryRowContext,(*DB).Exec,(*DB).ExecContext,(*DB).Query,(*DB).QueryContext,(*DB).QueryRow,(*DB).QueryRowContext,(*Stmt).Exec,(*Stmt).ExecContext,(*Stmt).Query,(*Stmt).QueryContext,(*Stmt).QueryRow,(*Stmt).QueryRowContext,(*Tx).Exec,(*Tx).ExecContext,(*Tx).Query,(*Tx).QueryContext,(*Tx).QueryRow,(*Tx).QueryRowContext assert x.(database/sql/driver.Valuer)
	// row: database/sql (*Conn).ExecContext,(*Conn).QueryContext,(*Conn).QueryRowContext,(*DB).Exec,(*DB).ExecContext,(*DB).Query,(*DB).QueryContext,(*DB).QueryRow,(*DB).QueryRowContext,(*Stmt).Exec,(*Stmt).ExecContext,(*Stmt).Query,(*Stmt).QueryContext,(*Stmt).QueryRow,(*Stmt).QueryRowContext,(*Tx).Exec,(*Tx).ExecContext,(*Tx).Query,(*Tx).QueryContext,(*Tx).QueryRow,(*Tx).QueryRowContext reflect kind
	// row: database/sql (*Conn).ExecContext,(*Conn).QueryContext,(*Conn).QueryRowContext,(*DB).Exec,(*DB).ExecContext,(*DB).Query,(*DB).QueryContext,(*DB).QueryRow,(*DB).QueryRowContext,(*Stmt).Exec,(*Stmt).ExecContext,(*Stmt).Query,(*Stmt).QueryContext,(*Stmt).QueryRow,(*Stmt).QueryRowContext,(*Tx).Exec,(*Tx).ExecContext,(*Tx).Query,(*Tx).QueryContext,(*Tx).QueryRow,(*Tx).QueryRowContext reflect method set
	// row: database/sql (*Conn).ExecContext,(*Conn).QueryContext,(*Conn).QueryRowContext,(*DB).Exec,(*DB).ExecContext,(*DB).Query,(*DB).QueryContext,(*DB).QueryRow,(*DB).QueryRowContext,(*Stmt).Exec,(*Stmt).ExecContext,(*Stmt).Query,(*Stmt).QueryContext,(*Stmt).QueryRow,(*Stmt).QueryRowContext,(*Tx).Exec,(*Tx).ExecContext,(*Tx).Query,(*Tx).QueryContext,(*Tx).QueryRow,(*Tx).QueryRowContext reflect walk
	probe("Valuer", func() {
		takeCalls() // the calls of a probe that panicked
		var iv driver.Valuer = vval{"iface"}
		args := []any{vval{"a"}, &pval{3}, (*vval)(nil), (*pval)(nil), gval[int64]{9}, (*gval[int64])(nil),
			myInt(5), myStr("s"), myBool(true), myFloat(1.5), myBytes("by"), dec{true, []byte{1}, -1},
			iv, sql.NullString{String: "ns", Valid: true}}
		for i, m := range []func(b *bareConn) driver.Conn{
			func(b *bareConn) driver.Conn { return b },
			func(b *bareConn) driver.Conn { return execCtxConn{b} },
		} {
			out("conn %d", i)
			c := newCfg()
			c.conn = m
			db := open(c)
			every(db, "q", args...)
			r, err := db.Exec("q", sql.Named("n", vval{"named"}), sql.Named("p", &pval{1}))
			res("DB.Exec named", r, err)
			for _, a := range []any{gval[myInt]{4}, badVal{}, errVal{}, plain{1}, []myInt{1}, &plain{2}, (*plain)(nil), gval[plain]{}} {
				r, err := db.Exec("q", a)
				res(fmt.Sprintf("DB.Exec %T", a), r, err)
			}
			done("DB.Close", fmt.Sprint(db.Close()))
		}
	})

	// row: database/sql (*DB).Exec,(*DB).Query assert x.(database/sql/driver.Valuer)
	probe("Valuer/NullT", func() {
		takeCalls() // the calls of a probe that panicked
		// sql.Null[T] of a type of the program is a Valuer of the std.
		c := newCfg()
		db := open(c)
		for _, a := range []any{sql.Null[myInt]{V: 4, Valid: true}, sql.Null[myStr]{V: "s", Valid: true},
			sql.Null[vval]{V: vval{"n"}, Valid: true}, sql.Null[myInt]{}, &sql.Null[myInt]{V: 5, Valid: true}} {
			r, err := db.Exec("q", a)
			res(fmt.Sprintf("DB.Exec %T", a), r, err)
			rs, err := db.Query("q", a)
			rowsRes(fmt.Sprintf("DB.Query %T", a), rs, err)
		}
		done("DB.Close", fmt.Sprint(db.Close()))
	})
	probe("Valuer/NullNoInstance", func() {
		takeCalls() // the calls of a probe that panicked
		// sql.Null[T] whose T has no native instance of Null in the VM: a
		// slice that is not []byte, a named []byte of the std.
		vals := []any{sql.Null[[]int]{V: []int{1}, Valid: true}, sql.Null[json.RawMessage]{V: json.RawMessage("1"), Valid: true},
			sql.Null[[]int]{}, &sql.Null[json.RawMessage]{V: json.RawMessage("2"), Valid: true}}
		for _, a := range vals {
			v, err := driver.DefaultParameterConverter.ConvertValue(a)
			out("ConvertValue %T: %T %v %v", a, v, v, err)
		}
		c := newCfg()
		db := open(c)
		for _, a := range vals {
			r, err := db.Exec("q", a)
			res(fmt.Sprintf("DB.Exec %T", a), r, err)
		}
		done("DB.Close", fmt.Sprint(db.Close()))
	})

	probe("Valuer/NullInterface", func() {
		// sql.Null[T] with an interface T: Value calls the Value of a
		// driver.Valuer in V, then converts its result (an int to int64).
		vals := []any{sql.Null[nullIfcValuer]{V: nullIfcV{3}, Valid: true}, sql.Null[nullIfcValuer]{V: nullIfcW{4}, Valid: true},
			sql.Null[nullIfcValuer]{Valid: true}, sql.Null[driver.Valuer]{V: nullIfcV{5}, Valid: true},
			sql.Null[driver.Valuer]{V: (*nullIfcV)(nil), Valid: true}, sql.Null[any]{V: nullIfcV{6}, Valid: true},
			sql.Null[any]{V: int8(7), Valid: true}, sql.Null[fmt.Stringer]{Valid: true},
			// a native value, Valuers of database/sql, chains of Valuers
			sql.Null[any]{V: io.EOF, Valid: true}, sql.Null[any]{V: sql.NullString{String: "ns", Valid: true}, Valid: true},
			sql.Null[driver.Valuer]{V: sql.NullBool{Bool: true, Valid: true}, Valid: true},
			sql.Null[any]{V: nullIfcChain{nullIfcV{9}}, Valid: true}, sql.Null[any]{V: nullIfcChain{sql.NullInt64{Int64: 10, Valid: true}}, Valid: true},
			sql.Null[any]{V: nullIfcChain{nullIfcChain{nullIfcV{11}}}, Valid: true}}
		for _, a := range vals {
			v, err := a.(driver.Valuer).Value()
			out("Value %T: %T %v %v", a, v, v, err)
		}
	})

	// row: database/sql (*Conn).ExecContext,(*Conn).QueryContext,(*Conn).QueryRowContext,(*DB).Exec,(*DB).ExecContext,(*DB).Query,(*DB).QueryContext,(*DB).QueryRow,(*DB).QueryRowContext,(*Stmt).Exec,(*Stmt).ExecContext,(*Stmt).Query,(*Stmt).QueryContext,(*Stmt).QueryRow,(*Stmt).QueryRowContext,(*Tx).Exec,(*Tx).ExecContext,(*Tx).Query,(*Tx).QueryContext,(*Tx).QueryRow,(*Tx).QueryRowContext assert x.(database/sql/driver.NamedValueChecker)
	// row: database/sql (*Conn).ExecContext,(*Conn).QueryContext,(*Conn).QueryRowContext,(*DB).Exec,(*DB).ExecContext,(*DB).Query,(*DB).QueryContext,(*DB).QueryRow,(*DB).QueryRowContext,(*Stmt).Exec,(*Stmt).ExecContext,(*Stmt).Query,(*Stmt).QueryContext,(*Stmt).QueryRow,(*Stmt).QueryRowContext,(*Tx).Exec,(*Tx).ExecContext,(*Tx).Query,(*Tx).QueryContext,(*Tx).QueryRow,(*Tx).QueryRowContext panic x.(database/sql/driver.NamedValueChecker) without ok: *runtime.TypeAssertionError
	probe("NamedValueChecker", func() {
		takeCalls() // the calls of a probe that panicked
		cases := []struct {
			conn func(b *bareConn) driver.Conn
			stmt func(s *bareStmt) driver.Stmt
		}{
			{func(b *bareConn) driver.Conn { return nvcConn{b} }, func(s *bareStmt) driver.Stmt { return s }},
			{func(b *bareConn) driver.Conn { return b }, func(s *bareStmt) driver.Stmt { return nvcStmt{s} }},
			{func(b *bareConn) driver.Conn { return nvcConn{b} }, func(s *bareStmt) driver.Stmt { return nvcccStmt{s} }},
		}
		for i, k := range cases {
			out("case %d", i)
			c := newCfg()
			c.conn, c.stmt = k.conn, k.stmt
			db := open(c)
			every(db, "q", myInt(2), "skip", "remove", vval{"v"}, plain{7})
			r, err := db.Exec("q", "bad")
			res("DB.Exec bad", r, err)
			done("DB.Close", fmt.Sprint(db.Close()))
		}
	})

	// row: database/sql (*Conn).ExecContext,(*Conn).QueryContext,(*Conn).QueryRowContext,(*DB).Exec,(*DB).ExecContext,(*DB).Query,(*DB).QueryContext,(*DB).QueryRow,(*DB).QueryRowContext,(*Stmt).Exec,(*Stmt).ExecContext,(*Stmt).Query,(*Stmt).QueryContext,(*Stmt).QueryRow,(*Stmt).QueryRowContext,(*Tx).Exec,(*Tx).ExecContext,(*Tx).Query,(*Tx).QueryContext,(*Tx).QueryRow,(*Tx).QueryRowContext assert x.(database/sql/driver.ColumnConverter)
	probe("ColumnConverter", func() {
		takeCalls() // the calls of a probe that panicked
		c := newCfg()
		c.stmt = func(s *bareStmt) driver.Stmt { return ccStmt{s} }
		c.nIn = 4
		db := open(c)
		every(db, "q", "x", vval{"v"}, 3, gval[int64]{8})
		for _, args := range [][]any{{myInt(1), 1, 2, 3}, {badVal{}, 1, 2, 3}, {1, 2}} {
			r, err := db.Exec("q", args...)
			res(fmt.Sprintf("DB.Exec %T", args[0]), r, err)
		}
		done("DB.Close", fmt.Sprint(db.Close()))
	})

	// row: database/sql (*Rows).ColumnTypes assert x.(database/sql/driver.RowsColumnTypeScanType)
	// row: database/sql (*Rows).ColumnTypes assert x.(database/sql/driver.RowsColumnTypeDatabaseTypeName)
	// row: database/sql (*Rows).ColumnTypes assert x.(database/sql/driver.RowsColumnTypeLength)
	// row: database/sql (*Rows).ColumnTypes assert x.(database/sql/driver.RowsColumnTypeNullable)
	// row: database/sql (*Rows).ColumnTypes assert x.(database/sql/driver.RowsColumnTypePrecisionScale)
	probe("ColumnTypes", func() {
		takeCalls() // the calls of a probe that panicked
		base := func() *bareRows {
			return &bareRows{cols: []string{"a", "b", "c"}, data: [][]driver.Value{{int64(1), "x", nil}}}
		}
		for i, mk := range []func() driver.Rows{
			func() driver.Rows { return base() },
			func() driver.Rows { return ctRows{base()} },
			func() driver.Rows { return gctRows[myStr]{base()} },
		} {
			c := newCfg()
			c.rows = mk
			db := open(c)
			rs, err := db.Query("q")
			if err != nil {
				out("%d: %v", i, err)
				continue
			}
			cts, err := rs.ColumnTypes()
			out("%d: err %v", i, err)
			for _, ct := range cts {
				l, lok := ct.Length()
				n, nok := ct.Nullable()
				p, s, dok := ct.DecimalSize()
				st := ct.ScanType()
				out("%d: %s scan %v name %q kind %v db %q len %d %v null %v %v dec %d %d %v",
					i, ct.Name(), st, st.Name(), st.Kind(), ct.DatabaseTypeName(), l, lok, n, nok, p, s, dok)
			}
			rs.Close()
			db.Close()
			takeCalls()
		}
	})

	// row: database/sql (*Row).Scan,(*Rows).Next,(*Rows).NextResultSet assert x.(database/sql/driver.RowsNextResultSet)
	probe("NextResultSet", func() {
		takeCalls() // the calls of a probe that panicked
		for i, mk := range []func() driver.Rows{
			func() driver.Rows {
				return &bareRows{cols: []string{"a"}, data: [][]driver.Value{{"r1"}, {"r2"}}}
			},
			func() driver.Rows {
				return &nrsRows{cols: []string{"a"}, sets: [][][]driver.Value{{{"s0r1"}, {"s0r2"}}, {}, {{"s2r1"}}}}
			},
			func() driver.Rows {
				return &nrsRows{cols: []string{"a"}, sets: [][][]driver.Value{{}, {{"x"}}}}
			},
		} {
			c := newCfg()
			c.rows = mk
			db := open(c)
			rs, err := db.Query("q")
			if err != nil {
				out("%d: %v", i, err)
				continue
			}
			takeCalls()
			for set := 0; ; set++ {
				var got []string
				for rs.Next() {
					var s string
					rs.Scan(&s)
					got = append(got, s)
				}
				more := rs.NextResultSet()
				done(fmt.Sprintf("%d set %d", i, set), fmt.Sprintf("%v more %v err %v", got, more, rs.Err()))
				if !more {
					break
				}
			}
			rs.Close()
			var s string
			err = db.QueryRow("q").Scan(&s)
			done(fmt.Sprintf("%d Row.Scan", i), fmt.Sprintf("%q %v", s, err))
			db.Close()
			takeCalls()
		}
	})

	scanCases := []scase{
		{src(int64(7)), dst[string]},
		{src(int64(7)), dst[[]byte]},
		{src(int64(7)), dst[sql.RawBytes]},
		{src(int64(300)), dst[int8]},
		{src(int64(7)), dst[myInt]},
		{src(int64(7)), dst[myStr]},
		{src(int64(7)), ptrDest},
		{src(int64(7)), dst[scn]},
		{src(int64(7)), dst[gscn[int64]]},
		{src(int64(7)), func() (any, func() string) { return vscn{"v"}, func() string { return takeCalls() } }},
		{src(int64(7)), func() (any, func() string) { return myInt(0), func() string { return "-" } }},
		{src(int64(7)), func() (any, func() string) { return (*int)(nil), func() string { return "-" } }},
		{src(int64(7)), dst[any]},
		{src(myInt(7)), dst[string]},
		{src(myInt(7)), dst[[]byte]},
		{src(myInt(7)), dst[sql.RawBytes]},
		{src(myInt(7)), dst[int]},
		{src(myInt(7)), dst[myInt]},
		{src(myInt(7)), dst[uint8]},
		{src(myInt(7)), dst[float64]},
		{src(myInt(7)), dst[any]},
		{src(myInt(7)), dst[bool]},
		{src(myInt(1)), dst[bool]},
		{src(myInt(7)), dst[scn]},
		{src(myInt(7)), dst[gscn[myInt]]},
		{src(myInt(7)), dst[sql.NullInt64]},
		{src(myStr("42")), dst[string]},
		{src(myStr("42")), dst[[]byte]},
		{src(myStr("42")), dst[sql.RawBytes]},
		{src(myStr("42")), dst[int]},
		{src(myStr("42")), dst[myStr]},
		{src(myStr("true")), dst[bool]},
		{src(myStr("42")), dst[sql.NullString]},
		{src(myBytes("xy")), dst[[]byte]},
		{src(myBytes("xy")), dst[string]},
		{src(myBytes("xy")), dst[myBytes]},
		{src(myBytes("xy")), dst[any]},
		{src(myBool(true)), dst[bool]},
		{src(myBool(true)), dst[string]},
		{src(myBool(true)), dst[int]},
		{src(myFloat(1.5)), dst[string]},
		{src(myFloat(1.5)), dst[float32]},
		{src(myFloat(1.5)), dst[int]},
		{src(myFloat(1.5)), dst[[]byte]},
		{src("s"), dst[myStr]},
		{src("s"), dst[scn]},
		{src("s"), dst[gscn[string]]},
		{src("12"), dst[myInt]},
		{src("s"), dst[int]},
		{src([]byte("b")), dst[string]},
		{src([]byte("b")), dst[myBytes]},
		{src([]byte("b")), dst[scn]},
		{src(nil), dst[any]},
		{src(nil), dst[[]byte]},
		{src(nil), dst[myInt]},
		{src(nil), ptrDest},
		{src(nil), dst[scn]},
		{src(nil), dst[gscn[string]]},
		{src(nil), dst[sql.NullString]},
		{src(dec{true, []byte{1, 2}, -2}), dst[dec]},
		{src(&dec{false, []byte{3}, 1}), dst[dec]},
		{src(dec{true, []byte{1, 2}, -2}), dst[string]},
		{src(dec{true, []byte{1, 2}, -2}), dst[scn]},
		{src(dec{true, []byte{1, 2}, -2}), dst[any]},
		{src(time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)), dst[myStr]},
		{cursor, rowsDest},
		{cursor, dst[string]},
		{func() driver.Value { return &plain{1} }, dst[plain]},
		{func() driver.Value { return &plain{1} }, dst[*plain]},
		{func() driver.Value { return &plain{1} }, dst[any]},
	}

	// row: database/sql (*Row).Scan,(*Rows).Scan assert x.(T), T unnamed
	// row: database/sql (*Row).Scan,(*Rows).Scan assert x.(database/sql.Scanner)
	// row: database/sql (*Row).Scan,(*Rows).Scan assert x.(database/sql.decimalCompose)
	// row: database/sql (*Row).Scan,(*Rows).Scan assert x.(database/sql.decimalDecompose)
	// row: database/sql (*Row).Scan,(*Rows).Scan assert x.(database/sql/driver.Rows)
	// row: database/sql (*Row).Scan,(*Rows).Scan panic x.(T) without ok, T unnamed: *runtime.TypeAssertionError
	// row: database/sql (*Row).Scan,(*Rows).Scan reflect kind
	// row: database/sql (*Row).Scan,(*Rows).Scan reflect type name
	// row: database/sql (*Row).Scan,(*Rows).Scan reflect walk
	probe("Scan", func() {
		takeCalls() // the calls of a probe that panicked
		scanAll(scanCases)
	})

	// row: database/sql (*NullBool).Scan,(*NullByte).Scan,(*NullFloat64).Scan,(*NullInt16).Scan,(*NullInt32).Scan,(*NullInt64).Scan,(*NullString).Scan,(*NullTime).Scan assert x.(T), T unnamed
	// row: database/sql (*NullBool).Scan,(*NullByte).Scan,(*NullFloat64).Scan,(*NullInt16).Scan,(*NullInt32).Scan,(*NullInt64).Scan,(*NullString).Scan,(*NullTime).Scan assert x.(database/sql.Scanner)
	// row: database/sql (*NullBool).Scan,(*NullByte).Scan,(*NullFloat64).Scan,(*NullInt16).Scan,(*NullInt32).Scan,(*NullInt64).Scan,(*NullString).Scan,(*NullTime).Scan assert x.(database/sql.decimalCompose)
	// row: database/sql (*NullBool).Scan,(*NullByte).Scan,(*NullFloat64).Scan,(*NullInt16).Scan,(*NullInt32).Scan,(*NullInt64).Scan,(*NullString).Scan,(*NullTime).Scan assert x.(database/sql.decimalDecompose)
	// row: database/sql (*NullBool).Scan,(*NullByte).Scan,(*NullFloat64).Scan,(*NullInt16).Scan,(*NullInt32).Scan,(*NullInt64).Scan,(*NullString).Scan,(*NullTime).Scan assert x.(database/sql/driver.Rows)
	// row: database/sql (*NullBool).Scan,(*NullByte).Scan,(*NullFloat64).Scan,(*NullInt16).Scan,(*NullInt32).Scan,(*NullInt64).Scan,(*NullString).Scan,(*NullTime).Scan panic x.(T) without ok, T unnamed: *runtime.TypeAssertionError
	// row: database/sql (*NullBool).Scan,(*NullByte).Scan,(*NullFloat64).Scan,(*NullInt16).Scan,(*NullInt32).Scan,(*NullInt64).Scan,(*NullString).Scan,(*NullTime).Scan reflect kind
	// row: database/sql (*NullBool).Scan,(*NullByte).Scan,(*NullFloat64).Scan,(*NullInt16).Scan,(*NullInt32).Scan,(*NullInt64).Scan,(*NullString).Scan,(*NullTime).Scan reflect type name
	// row: database/sql (*NullBool).Scan,(*NullByte).Scan,(*NullFloat64).Scan,(*NullInt16).Scan,(*NullInt32).Scan,(*NullInt64).Scan,(*NullString).Scan,(*NullTime).Scan reflect walk
	probe("NullScan", func() {
		takeCalls() // the calls of a probe that panicked
		srcs := []any{myInt(5), myStr("6"), myBool(true), myFloat(2.5), myBytes("7"), dec{false, []byte{9}, 0},
			cursor(), nil, int64(1), "x", time.Date(2021, 2, 3, 4, 5, 6, 0, time.UTC), gval[int]{1}}
		type scanner interface{ Scan(any) error }
		mks := []func() scanner{
			func() scanner { return &sql.NullBool{} },
			func() scanner { return &sql.NullByte{} },
			func() scanner { return &sql.NullFloat64{} },
			func() scanner { return &sql.NullInt16{} },
			func() scanner { return &sql.NullInt32{} },
			func() scanner { return &sql.NullInt64{} },
			func() scanner { return &sql.NullString{} },
			func() scanner { return &sql.NullTime{} },
		}
		for _, mk := range mks {
			for _, s := range srcs {
				n := mk()
				err := n.Scan(s)
				out("%T <- %T: %+v %v", n, s, n, err)
			}
		}
	})

	// row: database/sql (*DB).Close assert x.(io.Closer)
	probe("DB.Close", func() {
		takeCalls() // the calls of a probe that panicked
		for i, k := range []driver.Connector{
			connector{newCfg()},
			closingConnector{connector{newCfg()}},
			&gcloser[string]{connector{newCfg()}, "g"},
		} {
			db := sql.OpenDB(k)
			err := db.Ping()
			done(fmt.Sprintf("%d %T ping %v close", i, k, err), fmt.Sprint(db.Close()))
		}
	})

	// row: database/sql (*Conn).BeginTx,(*Conn).Close,(*Conn).ExecContext,(*Conn).PingContext,(*Conn).PrepareContext,(*Conn).QueryContext,(*Conn).QueryRowContext,(*Conn).Raw,(*DB).Begin,(*DB).BeginTx,(*DB).Close,(*DB).Conn,(*DB).Exec,(*DB).ExecContext,(*DB).Ping,(*DB).PingContext,(*DB).Prepare,(*DB).PrepareContext,(*DB).Query,(*DB).QueryContext,(*DB).QueryRow,(*DB).QueryRowContext,(*DB).SetConnMaxIdleTime,(*DB).SetConnMaxLifetime,(*DB).SetMaxIdleConns,(*DB).SetMaxOpenConns,(*Stmt).Close,(*Stmt).Exec,(*Stmt).ExecContext,(*Stmt).Query,(*Stmt).QueryContext,(*Stmt).QueryRow,(*Stmt).QueryRowContext,(*Tx).Commit,(*Tx).Prepare,(*Tx).PrepareContext,(*Tx).Rollback,(*Tx).Stmt,(*Tx).StmtContext,Open,OpenDB panic map key of interface type: hash of unhashable type
	// row: database/sql (*Conn).BeginTx,(*Conn).Close,(*Conn).ExecContext,(*Conn).PingContext,(*Conn).PrepareContext,(*Conn).QueryContext,(*Conn).QueryRowContext,(*Conn).Raw,(*DB).Begin,(*DB).BeginTx,(*DB).Close,(*DB).Conn,(*DB).Exec,(*DB).ExecContext,(*DB).Ping,(*DB).PingContext,(*DB).Prepare,(*DB).PrepareContext,(*DB).Query,(*DB).QueryContext,(*DB).QueryRow,(*DB).QueryRowContext,(*DB).SetConnMaxIdleTime,(*DB).SetConnMaxLifetime,(*DB).SetMaxIdleConns,(*DB).SetMaxOpenConns,(*Stmt).Close,(*Stmt).Exec,(*Stmt).ExecContext,(*Stmt).Query,(*Stmt).QueryContext,(*Stmt).QueryRow,(*Stmt).QueryRowContext,(*Tx).Commit,(*Tx).Rollback,Open,OpenDB panic panic message with the type name
	probe("Lifecycle", func() {
		takeCalls() // the calls of a probe that panicked
		c := newCfg()
		c.conn = func(b *bareConn) driver.Conn { return resetConn{b} }
		sql.Register("boundary-database-sql", drv{c})
		gc := newCfg()
		sql.Register("boundary-database-sql-g", gdrv[int]{gc, 7})
		var found []string
		for _, n := range sql.Drivers() {
			if strings.HasPrefix(n, "boundary-database-sql") {
				found = append(found, n)
			}
		}
		out("drivers %v", found)
		gdb, err := sql.Open("boundary-database-sql-g", "gdsn")
		if err == nil {
			done("gdrv ping", fmt.Sprint(gdb.Ping(), gdb.Close()))
		}
		_, err = sql.Open("boundary-database-sql-none", "x")
		out("unknown: %v", err)
		db, err := sql.Open("boundary-database-sql", "dsn")
		if err != nil {
			out("open: %v", err)
			return
		}
		db.SetConnMaxIdleTime(time.Hour)
		db.SetConnMaxLifetime(time.Hour)
		db.SetMaxIdleConns(2)
		db.SetMaxOpenConns(1)
		out("driver %T", db.Driver())
		every(db, "q", 1)
		conn, err := db.Conn(ctx)
		if err == nil {
			t, err := conn.BeginTx(ctx, nil)
			if err == nil {
				t.Commit()
			}
			done("Conn.PingContext", fmt.Sprint(conn.PingContext(ctx)))
			st, err := conn.PrepareContext(ctx, "cp")
			if err == nil {
				done("Stmt.Close", fmt.Sprint(st.Close()))
			}
			conn.Raw(func(dc any) error { out("Raw %T", dc); return nil })
			done("Conn.Close", fmt.Sprint(conn.Close()))
		}
		done("DB.Ping", fmt.Sprint(db.Ping(), db.PingContext(ctx)))
		st, err := db.PrepareContext(ctx, "s")
		if err == nil {
			t, err := db.BeginTx(ctx, nil)
			if err == nil {
				t.Stmt(st).Close()
				t.StmtContext(ctx, st).Close()
				if p, err := t.Prepare("tp"); err == nil {
					p.Close()
				}
				if p, err := t.PrepareContext(ctx, "tpc"); err == nil {
					p.Close()
				}
				done("Tx.Rollback", fmt.Sprint(t.Rollback()))
			}
			st.Close()
		}
		done("DB.Close", fmt.Sprint(db.Close()))
	})

	// row: database/sql/driver IsValue,IsScanValue assert x.(T), T unnamed
	// row: database/sql/driver IsValue,IsScanValue assert x.(database/sql/driver.decimalDecompose)
	probe("IsValue", func() {
		takeCalls() // the calls of a probe that panicked
		for _, v := range []any{nil, int64(1), myInt(1), "s", myStr("s"), []byte("b"), myBytes("b"), true, myBool(true),
			1.5, myFloat(1.5), time.Time{}, dec{}, &dec{}, gval[int64]{1}, vval{}, plain{}} {
			out("%T: %v %v", v, driver.IsValue(v), driver.IsScanValue(v))
		}
	})
}

type nullIfcValuer interface {
	Value() (driver.Value, error)
}

type nullIfcV struct{ n int }

func (v nullIfcV) Value() (driver.Value, error) { return v.n, nil }

type nullIfcW struct{ n int64 }

func (v nullIfcW) Value() (driver.Value, error) { return v.n, nil }

type nullIfcChain struct{ v driver.Value }

func (c nullIfcChain) Value() (driver.Value, error) { return c.v, nil }
