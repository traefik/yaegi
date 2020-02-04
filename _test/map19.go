package main

import "fmt"

type cmap struct {
	servers map[int64]*server
}

type server struct {
	cm *cmap
}

func main() {
	//s := &server{}
	//m := cmap{servers: map[int64]*server{1: s}}
	m := cmap{}
	fmt.Println(m)
}
