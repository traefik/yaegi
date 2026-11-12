// sync.Mutex and sync.RWMutex: counters updated by many goroutines (only the
// final value is printed), TryLock, TryRLock, a mutex embedded in a struct,
// Locker interface, and unlock of an unlocked mutex (fatal, so not done).
package main

import (
	"fmt"
	"sync"
)

type SafeCounter struct {
	sync.Mutex
	m map[string]int
}

func (c *SafeCounter) Inc(k string) {
	c.Lock()
	defer c.Unlock()
	c.m[k]++
}

type Store struct {
	mu   sync.RWMutex
	data map[int]int
}

func (s *Store) Get(k int) (int, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[k]
	return v, ok
}

func (s *Store) Set(k, v int) {
	s.mu.Lock()
	s.data[k] = v
	s.mu.Unlock()
}

func main() {
	var mu sync.Mutex
	var wg sync.WaitGroup
	n := 0
	var n64 int64
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				mu.Lock()
				n++
				n64 += int64(j)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	fmt.Println(n, n64)

	sc := SafeCounter{m: map[string]int{}}
	keys := []string{"a", "b", "c"}
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sc.Inc(keys[i%3])
		}()
	}
	wg.Wait()
	fmt.Println(sc.m)

	st := &Store{data: map[int]int{}}
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			st.Set(i, i*i)
		}()
	}
	wg.Wait()
	var readers sync.WaitGroup
	sums := make([]int, 5)
	for r := 0; r < 5; r++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for k := 0; k < 20; k++ {
				v, _ := st.Get(k)
				sums[r] += v
			}
		}()
	}
	readers.Wait()
	fmt.Println(sums)
	_, ok := st.Get(100)
	fmt.Println("missing", ok)

	// TryLock.
	var t sync.Mutex
	fmt.Println("try1", t.TryLock())
	fmt.Println("try2", t.TryLock())
	t.Unlock()
	fmt.Println("try3", t.TryLock())
	t.Unlock()

	var rw sync.RWMutex
	fmt.Println("tryR1", rw.TryRLock())
	fmt.Println("tryR2", rw.TryRLock())
	fmt.Println("tryW while readers", rw.TryLock())
	rw.RUnlock()
	rw.RUnlock()
	fmt.Println("tryW", rw.TryLock())
	fmt.Println("tryR while writer", rw.TryRLock())
	rw.Unlock()

	// Locker interface and RLocker.
	var l sync.Locker = &mu
	l.Lock()
	l.Unlock()
	rl := rw.RLocker()
	rl.Lock()
	fmt.Println("writer blocked by RLocker", rw.TryLock())
	rl.Unlock()
	fmt.Println("writer free", rw.TryLock())
	rw.Unlock()
	fmt.Printf("%T %T\n", l, rl)

	// A mutex hand-off: goroutine waits for the lock held by main.
	var h sync.Mutex
	h.Lock()
	got := make(chan string)
	go func() {
		h.Lock()
		got <- "goroutine has the lock"
		h.Unlock()
	}()
	h.Unlock()
	fmt.Println(<-got)
}
