package main

import (
	"encoding/xml"
	"errors"
	"fmt"
)

type Email struct {
	Where string `xml:"where,attr"`
	Addr  string
}

func f(r any) error {
	return withPointerAsInterface(&r)
}

func withPointerAsInterface(r any) error {
	_ = (r).(*any)
	rp, ok := (r).(*any)
	if !ok {
		return errors.New("cannot assert to *interface{}")
	}
	em, ok := (*rp).(*Email)
	if !ok {
		return errors.New("cannot assert to *Email")
	}
	em.Where = "work"
	em.Addr = "bob@work.com"
	return nil
}

func ff(s string, r any) error {
	return xml.Unmarshal([]byte(s), r)
}

func fff(s string, r any) error {
	return xml.Unmarshal([]byte(s), &r)
}

func main() {
	data := `
		<Email where='work'>
			<Addr>bob@work.com</Addr>
		</Email>
	`
	v := Email{}
	err := f(&v)
	fmt.Println(err, v)

	vv := Email{}
	err = ff(data, &vv)
	fmt.Println(err, vv)

	vvv := Email{}
	err = ff(data, &vvv)
	fmt.Println(err, vvv)
}

// Output:
// <nil> {work bob@work.com}
// <nil> {work bob@work.com}
// <nil> {work bob@work.com}
