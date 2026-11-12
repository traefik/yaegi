package main

import (
	"fmt"

	"github.com/Masterminds/semver/v3"
)

func main() {
	c, _ := semver.NewConstraint(">= 1.2, < 3")
	for _, s := range []string{"1.1.0", "2.5.1", "3.0.0-rc1"} {
		v := semver.MustParse(s)
		fmt.Println(v, c.Check(v), v.Major())
	}
}
