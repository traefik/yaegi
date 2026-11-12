// json: MarshalJSON and UnmarshalJSON on the top-level value
// (value and pointer receivers).
package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Celsius float64

func (c Celsius) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%.1fC"`, float64(c))), nil
}

func (c *Celsius) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"C`)
	var f float64
	_, err := fmt.Sscanf(s, "%g", &f)
	*c = Celsius(f)
	return err
}

type Point struct{ X, Y int }

func (p *Point) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`[%d,%d]`, p.X, p.Y)), nil
}

func (p *Point) UnmarshalJSON(b []byte) error {
	var a [2]int
	if err := json.Unmarshal(b, &a); err != nil {
		return err
	}
	p.X, p.Y = a[0], a[1]
	return nil
}

func main() {
	b, err := json.Marshal(Celsius(21.5))
	fmt.Println(string(b), err)
	var c Celsius
	err = json.Unmarshal([]byte(`"18.2C"`), &c)
	fmt.Println(float64(c), err)

	b, err = json.Marshal(&Point{1, 2})
	fmt.Println(string(b), err)
	// A Point value: the pointer method is not in the method set.
	b, err = json.Marshal(Point{3, 4})
	fmt.Println(string(b), err)
	var p Point
	err = json.Unmarshal([]byte(`[5,6]`), &p)
	fmt.Println(p.X, p.Y, err)

	b, err = json.MarshalIndent(&Point{7, 8}, "", "  ")
	fmt.Println(string(b), err)

	// Errors returned by the method.
	err = json.Unmarshal([]byte(`{"a":1}`), &p)
	fmt.Println(err != nil)
}
