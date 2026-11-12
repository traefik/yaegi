package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Level int

func (l Level) MarshalJSON() ([]byte, error) { return []byte(fmt.Sprintf(`"L%d"`, int(l))), nil }

func (l *Level) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	_, err := fmt.Sscanf(s, "L%d", (*int)(l))
	return err
}

type Dur struct{ time.Duration }

func (d *Dur) UnmarshalText(b []byte) (err error) {
	d.Duration, err = time.ParseDuration(string(b))
	return
}

func (d Dur) MarshalText() ([]byte, error) { return []byte(strings.ToUpper(d.String())), nil }

type Cfg struct {
	Level   Level         `json:"level"`
	Timeout Dur           `json:"timeout"`
	ByLevel map[Dur]Level `json:"by,omitempty"`
}

func main() {
	b, err := json.Marshal(Cfg{Level: 2, Timeout: Dur{3 * time.Second}})
	fmt.Println(string(b), err)
	var c Cfg
	err = json.Unmarshal([]byte(`{"level":"L5","timeout":"1m30s"}`), &c)
	fmt.Println(err, int(c.Level), c.Timeout.Duration)
	b, err = json.MarshalIndent([]Level{1, 2}, "", " ")
	fmt.Println(string(b), err)
}
