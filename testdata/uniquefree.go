package main

// unique.Make keeps a value only while a handle of it is alive, as Go's
// unique package does (weak references). A long-running plugin that interns
// one new value per request must not keep all of them in memory.
//
// 20000 distinct values of 8 KiB are interned and dropped: about 160 MB if
// they all stay in memory. The heap must stay far below that.

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unique"
)

func heap() uint64 {
	// Two collections, with a pause in between: the first one frees the
	// values, the cleanups then remove the map entries.
	runtime.GC()
	time.Sleep(10 * time.Millisecond)
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.HeapAlloc
}

func main() {
	pad := strings.Repeat("x", 8<<10)
	keep := unique.Make("kept" + pad)
	before := heap()
	for round := 0; round < 200; round++ {
		for i := 0; i < 100; i++ {
			h := unique.Make(strconv.Itoa(round*100+i) + pad)
			if len(h.Value()) != len(pad)+len(strconv.Itoa(round*100+i)) {
				panic("bad value")
			}
		}
		if round%20 == 0 {
			heap()
		}
	}
	after := heap()
	grow := int64(after) - int64(before)
	fmt.Println("heap growth below 40 MB:", grow < 40<<20)
	fmt.Println("kept handle still equal:", keep == unique.Make("kept"+pad), len(keep.Value()))
}
