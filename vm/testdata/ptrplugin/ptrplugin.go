// Package ptrplugin compares pointers and uses them as map keys in its
// init: TestLoadReleasesTypes checks that this does not keep the types of
// the plugin alive after the plugin is gone.
package ptrplugin

type node struct {
	next *node
	arr  *[2]int
	zero *struct{}
}

type key struct{ p *node }

// Count is set by init.
var Count int

func init() {
	l := &node{next: &node{}, arr: new([2]int), zero: new(struct{})}
	for n := l; n != nil; n = n.next {
		Count++
	}
	byPtr := map[*node]int{l: 1}
	byAny := map[any]int{l.next: 2, l.arr: 3}
	byKey := map[key]int{{l}: 4}
	var a, b any = l.arr, l.zero
	if l.next != l && byPtr[l] == 1 && byAny[l.arr] == 3 && byKey[key{l}] == 4 && a != b {
		Count += 10
	}
}
