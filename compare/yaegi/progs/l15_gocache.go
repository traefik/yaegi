package main

import (
	"fmt"
	"time"

	"github.com/patrickmn/go-cache"
)

type Entry struct{ V int }

func main() {
	c := cache.New(time.Minute, 10*time.Minute)
	c.Set("a", &Entry{1}, cache.DefaultExpiration)
	c.Set("b", Entry{2}, cache.NoExpiration)
	x, ok := c.Get("a")
	fmt.Println(x.(*Entry).V, ok)
	y, _ := c.Get("b")
	fmt.Println(y.(Entry).V, c.ItemCount())
	fmt.Println(c.Increment("n", 1))
	c.Set("n", 5, 0)
	fmt.Println(c.IncrementInt("n", 2))
}
