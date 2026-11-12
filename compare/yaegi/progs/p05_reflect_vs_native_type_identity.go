package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"time"
)

type Cfg struct {
	D time.Duration
	T time.Time
}

func main() {
	var c Cfg
	t := reflect.TypeOf(c)
	fmt.Println(t.Field(0).Type == reflect.TypeOf(time.Duration(0)), t.Field(1).Type == reflect.TypeOf(time.Time{}))
	fmt.Println(reflect.TypeOf(c) == reflect.TypeOf(Cfg{}))
	var x any = c
	_, ok := x.(Cfg)
	fmt.Println(ok)
	var raw json.RawMessage
	fmt.Println(reflect.TypeOf(raw).String(), reflect.TypeOf(&raw).Elem().Kind())
	fmt.Println(reflect.ValueOf(c).Field(0).Interface().(time.Duration))
}
