// reflect.Value.Cap of channels, slices, arrays and pointers to arrays, also
// with elements of a program type.
package main

import (
	"fmt"
	"reflect"
)

type K struct{ A int }

func main() {
	ch := reflect.MakeChan(reflect.TypeOf(make(chan K)), 2)
	fmt.Println(ch.Len(), ch.Cap())
	ch.Send(reflect.ValueOf(K{1}))
	fmt.Println(ch.Len(), ch.Cap())
	ch2 := reflect.MakeChan(reflect.TypeOf(make(chan int)), 3)
	fmt.Println(ch2.Len(), ch2.Cap(), cap(ch2.Interface().(chan int)))
	c := make(chan K, 4)
	c <- K{2}
	fmt.Println(reflect.ValueOf(c).Len(), reflect.ValueOf(c).Cap(), reflect.ValueOf(make([]int, 1, 5)).Cap())
	fmt.Println(reflect.ValueOf(make([]K, 2, 7)).Cap(), reflect.ValueOf([]byte("ab")[:1]).Cap() >= 1)
	fmt.Println(reflect.ValueOf([3]K{}).Cap(), reflect.ValueOf(&[4]int{}).Cap(), reflect.ValueOf(make(chan *K)).Cap())
	var nilc chan K
	var nils []K
	fmt.Println(reflect.ValueOf(nilc).Cap(), reflect.ValueOf(nils).Cap())
	func() {
		defer func() { fmt.Println(recover()) }()
		reflect.ValueOf(map[int]int{}).Cap()
	}()
	func() {
		defer func() { fmt.Println(recover()) }()
		reflect.ValueOf(K{}).Cap()
	}()
	func() {
		defer func() { fmt.Println(recover()) }()
		reflect.ValueOf(&K{}).Cap()
	}()
	func() {
		defer func() { fmt.Println(recover()) }()
		reflect.ValueOf(new(int)).Cap()
	}()
}
