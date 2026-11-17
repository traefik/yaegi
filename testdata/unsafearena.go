// The arena of the skiplist of badger (dgraph-io/badger/v4/skl): nodes of
// plain data, with sync/atomic fields, laid over a []byte made by make, and
// found again by their offset.
package main

import (
	"fmt"
	"sync/atomic"
	"time"
	"unsafe"
)

const maxHeight = 4

type node struct {
	value     atomic.Uint64
	keyOffset uint32
	keySize   uint16
	height    uint16
	tower     [maxHeight]atomic.Uint32
}

type arena struct {
	n   atomic.Uint32
	buf []byte
}

const nodeAlign = int(unsafe.Sizeof(uint64(0))) - 1

// arenas keeps the arenas on the heap, as in badger: a []byte on the stack
// is not aligned for the atomic fields.
var arenas []*arena

func newArena(n int) *arena {
	a := &arena{buf: make([]byte, n)}
	a.n.Store(1)
	arenas = append(arenas, a)
	return a
}

func (a *arena) putNode(height int) uint32 {
	unused := (maxHeight - height) * int(unsafe.Sizeof(uint32(0)))
	l := uint32(int(unsafe.Sizeof(node{})) - unused + nodeAlign)
	n := a.n.Add(l)
	return (n - l + uint32(nodeAlign)) &^ uint32(nodeAlign)
}

func (a *arena) getNode(off uint32) *node {
	if off == 0 {
		return nil
	}
	return (*node)(unsafe.Pointer(&a.buf[off]))
}

func (a *arena) getNodeOffset(nd *node) uint32 {
	if nd == nil {
		return 0
	}
	return uint32(uintptr(unsafe.Pointer(nd)) - uintptr(unsafe.Pointer(&a.buf[0])))
}

func main() {
	a := newArena(1 << 10)
	var offs []uint32
	for h := 1; h <= maxHeight; h++ {
		off := a.putNode(h)
		nd := a.getNode(off)
		nd.value.Store(uint64(h)<<32 | uint64(h*10))
		nd.keyOffset = uint32(100 + h)
		nd.keySize = uint16(h)
		nd.height = uint16(h)
		offs = append(offs, off)
	}
	// Link the nodes on each level, as the skiplist does with CompareAndSwap.
	for i := 0; i+1 < len(offs); i++ {
		nd := a.getNode(offs[i])
		for lvl := 0; lvl < int(nd.height); lvl++ {
			if !nd.tower[lvl].CompareAndSwap(0, offs[i+1]) {
				panic("cas")
			}
		}
	}
	for _, off := range offs {
		nd := a.getNode(off)
		next := []uint32{}
		for lvl := 0; lvl < int(nd.height); lvl++ {
			next = append(next, nd.tower[lvl].Load())
		}
		fmt.Println(off, a.getNodeOffset(nd) == off, nd.value.Load(), nd.keyOffset, nd.keySize, nd.height, next)
	}
	fmt.Println(a.getNode(0) == nil, a.getNodeOffset(nil))
	// The bytes are the bytes of the nodes.
	fmt.Println(a.buf[offs[0]+8], a.buf[offs[1]+8])

	// A struct with a field of a native type, copied in and out of the bytes.
	off := a.putNode(maxHeight)
	e := (*entry)(unsafe.Pointer(&a.buf[off]))
	*e = entry{ttl: 3 * time.Second, size: 42}
	cp := *e
	cp.ttl += time.Millisecond
	e2 := (*entry)(unsafe.Pointer(&a.buf[off+16]))
	*e2 = cp
	fmt.Println(e.ttl, e.size, e2.ttl, e2.size, cp == *e2)
}

type entry struct {
	ttl  time.Duration
	size uint32
}
