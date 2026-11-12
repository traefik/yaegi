package main

import (
	"fmt"
	"sync"
)

type Store struct {
	mu   sync.Mutex
	rw   sync.RWMutex
	once sync.Once
	n    int
	m    map[string]int
}

func (s *Store) Inc() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.n++
}

func (s *Store) Put(k string) {
	s.once.Do(func() { s.m = map[string]int{} })
	s.rw.Lock()
	s.m[k]++
	s.rw.Unlock()
}

func main() {
	s := &Store{}
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				s.Inc()
				s.Put("k")
			}
		}()
	}
	wg.Wait()
	s.rw.RLock()
	fmt.Println(s.n, s.m["k"])
	s.rw.RUnlock()
}
