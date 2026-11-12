// AOT test program: net/rpc serving an interpreted service over an
// in-memory connection (gob codec).
//
//aot:lib net/rpc encoding/gob
package main

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/rpc"
	"os"
	"strconv"
	"strings"
	"time"
)

type Args struct {
	A, B int
}

type Quotient struct {
	Quo, Rem int
}

type Arith int

func (t *Arith) Multiply(args *Args, reply *int) error {
	*reply = args.A * args.B
	return nil
}

func (t *Arith) Divide(args *Args, quo *Quotient) error {
	if args.B == 0 {
		return errors.New("divide by zero")
	}
	quo.Quo = args.A / args.B
	quo.Rem = args.A % args.B
	return nil
}

func (t *Arith) unexported(args *Args, reply *int) error { return nil }

type Echo struct{ prefix string }

func (e Echo) Say(s string, reply *string) error {
	*reply = e.prefix + strings.ToUpper(s)
	return nil
}

func dial() *rpc.Client {
	srv := rpc.NewServer()
	if err := srv.Register(new(Arith)); err != nil {
		fmt.Println("register:", err)
	}
	if err := srv.RegisterName("Echo", Echo{"> "}); err != nil {
		fmt.Println("register:", err)
	}
	fmt.Println(srv.Register(struct{}{}))
	c1, c2 := net.Pipe()
	go srv.ServeConn(c1)
	return rpc.NewClient(c2)
}

func main() {
	if benchMain(benchOp) {
		return
	}
	log.SetFlags(0)
	log.SetOutput(os.Stdout)
	cl := dial()
	defer cl.Close()
	var prod int
	err := cl.Call("Arith.Multiply", &Args{7, 8}, &prod)
	fmt.Println(err, prod)
	var q Quotient
	err = cl.Call("Arith.Divide", Args{17, 5}, &q)
	fmt.Println(err, q)
	err = cl.Call("Arith.Divide", Args{1, 0}, &q)
	fmt.Println(err)
	var s string
	err = cl.Call("Echo.Say", "hello", &s)
	fmt.Println(err, s)
	call := cl.Go("Arith.Multiply", &Args{3, 4}, new(int), nil)
	<-call.Done
	fmt.Println(call.Error, *call.Reply.(*int))
	err = cl.Call("Arith.Nope", &Args{}, &prod)
	fmt.Println(err)
}

var benchClient *rpc.Client

// benchOp: one synchronous call over the pipe.
func benchOp() {
	if benchClient == nil {
		benchClient = dial()
	}
	var prod int
	if err := benchClient.Call("Arith.Multiply", &Args{7, 8}, &prod); err != nil || prod != 56 {
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
