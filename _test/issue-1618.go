package main

import (
	"fmt"
	"runtime"
	"sync"
)

func humanizeBytes(bytes uint64) string {
	const (
		_         = iota
		kB uint64 = 1 << (10 * iota)
		mB
		gB
		tB
		pB
	)

	switch {
	case bytes < kB:
		return fmt.Sprintf("%dB", bytes)
	case bytes < mB:
		return fmt.Sprintf("%.2fKB", float64(bytes)/float64(kB))
	case bytes < gB:
		return fmt.Sprintf("%.2fMB", float64(bytes)/float64(mB))
	case bytes < tB:
		return fmt.Sprintf("%.2fGB", float64(bytes)/float64(gB))
	case bytes < pB:
		return fmt.Sprintf("%.2fTB", float64(bytes)/float64(tB))
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

func main() {
	i := 0
	wg := sync.WaitGroup{}

	for {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		fmt.Printf("#%d: alloc = %s, routines = %d, gc = %d\n", i, humanizeBytes(m.Alloc), runtime.NumGoroutine(), m.NumGC)

		wg.Add(1)
		go func() {
			wg.Done()
		}()
		wg.Wait()
		i = i + 1
	}
}
