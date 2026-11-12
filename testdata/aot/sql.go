// AOT test program: database/sql with an interpreted driver and
// interpreted Scanner / Valuer types.
//
//aot:lib database/sql database/sql/driver
package main

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

// ---- a tiny in-memory driver (interpreted) ----

type memDriver struct{}

func (memDriver) Open(name string) (driver.Conn, error) { return &memConn{}, nil }

type memConn struct{}

func (c *memConn) Prepare(query string) (driver.Stmt, error) { return &memStmt{q: query}, nil }
func (c *memConn) Close() error                              { return nil }
func (c *memConn) Begin() (driver.Tx, error)                 { return memTx{}, nil }

type memTx struct{}

func (memTx) Commit() error   { return nil }
func (memTx) Rollback() error { return nil }

type memStmt struct{ q string }

func (s *memStmt) Close() error  { return nil }
func (s *memStmt) NumInput() int { return strings.Count(s.q, "?") }
func (s *memStmt) Exec(args []driver.Value) (driver.Result, error) {
	return driver.RowsAffected(len(args)), nil
}
func (s *memStmt) Query(args []driver.Value) (driver.Rows, error) {
	if strings.Contains(s.q, "fail") {
		return nil, errors.New("query failed")
	}
	var echo []string
	for _, a := range args {
		echo = append(echo, fmt.Sprintf("%T:%v", a, a))
	}
	return &memRows{data: [][]driver.Value{
		{int64(1), "alice", []byte("blob"), 2.5, true, time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC), strings.Join(echo, ",")},
		{int64(2), "bob", nil, -1.0, false, nil, ""},
	}}, nil
}

type memRows struct {
	data [][]driver.Value
	i    int
}

func (r *memRows) Columns() []string { return []string{"id", "name", "blob", "score", "ok", "at", "echo"} }
func (r *memRows) Close() error      { return nil }
func (r *memRows) Next(dest []driver.Value) error {
	if r.i >= len(r.data) {
		return io.EOF
	}
	copy(dest, r.data[r.i])
	r.i++
	return nil
}

// ---- interpreted value types ----

// Name implements sql.Scanner.
type Name struct{ First string }

func (n *Name) Scan(src any) error {
	switch v := src.(type) {
	case string:
		n.First = strings.Title(v)
	case []byte:
		n.First = strings.Title(string(v))
	default:
		return fmt.Errorf("bad name %T", src)
	}
	return nil
}

// Cents implements driver.Valuer.
type Cents int

func (c Cents) Value() (driver.Value, error) { return int64(c) * 100, nil }

type Score float64

type ID int32

func main() {
	if benchMain(benchOp) {
		return
	}
	sql.Register("mem", memDriver{})
	db, err := sql.Open("mem", "x")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	rows, err := db.QueryContext(context.Background(), "select ? ? ?", Cents(3), "s", ID(4))
	if err != nil {
		fmt.Println("query:", err)
		return
	}
	for rows.Next() {
		var (
			id    ID
			name  Name
			blob  []byte
			score Score
			ok    bool
			at    sql.NullTime
			echo  sql.NullString
			ns    *string
		)
		_ = ns
		if err := rows.Scan(&id, &name, &blob, &score, &ok, &at, &echo); err != nil {
			fmt.Println("scan:", err)
			continue
		}
		fmt.Println(id, name.First, string(blob), score, ok, at.Valid, at.Time.Year(), echo.String)
	}
	fmt.Println(rows.Err(), rows.Close())
	var s string
	var i int
	err = db.QueryRow("select").Scan(&i, &s, new(any), new(float32), new(string), new(any), new(string))
	fmt.Println(err, i, s)
	err = db.QueryRow("select").Scan(&s)
	fmt.Println(err)
	var bad struct{}
	err = db.QueryRow("select").Scan(&bad, &s, &s, &s, &s, &s, &s)
	fmt.Println(err)
	_, err = db.Query("fail")
	fmt.Println(err)
	res, err := db.Exec("insert ? ?", Cents(1), 2)
	n, _ := res.RowsAffected()
	fmt.Println(err, n)
	_, err = db.Exec("insert ?", struct{}{})
	fmt.Println(err)
	tx, _ := db.Begin()
	fmt.Println(tx.Commit())
}

var benchDB *sql.DB

// benchOp: one query scanning a row into interpreted types.
func benchOp() {
	if benchDB == nil {
		sql.Register("membench", memDriver{})
		benchDB, _ = sql.Open("membench", "")
	}
	var (
		id    ID
		name  Name
		blob  []byte
		score Score
		ok    bool
		at    sql.NullTime
		echo  string
	)
	if err := benchDB.QueryRow("select").Scan(&id, &name, &blob, &score, &ok, &at, &echo); err != nil {
		panic(err)
	}
}

func benchMain(op func()) bool {
	n, _ := strconv.Atoi(os.Getenv("AOT_BENCH"))
	if n <= 0 {
		return false
	}
	op()
	t0 := time.Now()
	for i := 0; i < n; i++ {
		op()
	}
	fmt.Printf("BENCH %d\n", time.Since(t0).Nanoseconds()/int64(n))
	return true
}
