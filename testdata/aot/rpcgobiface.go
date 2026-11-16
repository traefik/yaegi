// AOT test program: net/rpc with gob values in interfaces, served by
// rpc.Accept. The server reads each connection on a goroutine that the copy
// of net/rpc starts itself (no interpreted code runs there), and the
// client reads the replies on a goroutine of its own: gob finds the names
// that the program registered (gob.Register) there too.
//
//aot:lib net/rpc encoding/gob
package main

import (
	"encoding/gob"
	"fmt"
	"net"
	"net/rpc"
)

type Shape interface{ Area() int }

type Rect struct{ W, H int }

func (r Rect) Area() int { return r.W * r.H }

type Args struct{ S Shape }

type Reply struct{ S Shape }

type Svc int

func (*Svc) Double(a *Args, r *Reply) error {
	rect := a.S.(Rect)
	r.S = Rect{W: rect.W * 2, H: rect.H}
	return nil
}

func main() {
	gob.Register(Rect{})
	srv := rpc.NewServer()
	if err := srv.Register(new(Svc)); err != nil {
		panic(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	go srv.Accept(ln)
	c, err := rpc.Dial("tcp", ln.Addr().String())
	if err != nil {
		panic(err)
	}
	defer c.Close()
	var r Reply
	if err := c.Call("Svc.Double", &Args{S: Rect{W: 3, H: 4}}, &r); err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("%T %v %d\n", r.S, r.S, r.S.Area())
}
