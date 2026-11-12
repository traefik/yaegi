// time.Duration without the clock: arithmetic, String of many values
// (sub-microsecond, negative, large), Hours/Minutes/Seconds/Milliseconds,
// Truncate, Round, Abs, ParseDuration and its errors, and constant overflow
// wrap with int64.
package main

import (
	"fmt"
	"time"
)

func main() {
	ds := []time.Duration{
		0, 1, 999, time.Microsecond, 1500 * time.Nanosecond, time.Millisecond,
		1234567 * time.Nanosecond, time.Second, 90 * time.Second, time.Hour,
		26*time.Hour + 3*time.Minute + 4*time.Second + 5*time.Millisecond,
		-time.Minute, -1, 1<<63 - 1, -1 << 63,
	}
	for _, d := range ds {
		fmt.Printf("%v|%s|%d|%.3f|%.3f|%d|%d|%d\n", d, d.String(), int64(d), d.Hours(), d.Minutes(), d.Milliseconds(), d.Microseconds(), d.Nanoseconds())
	}
	d := 3*time.Hour + 25*time.Minute + 45*time.Second + 500*time.Millisecond
	fmt.Println(d, d.Seconds())
	for _, m := range []time.Duration{0, -1, time.Nanosecond, time.Second, time.Minute, time.Hour, 7 * time.Minute} {
		fmt.Println("trunc", m, d.Truncate(m), "round", d.Round(m))
	}
	fmt.Println((-d).Truncate(time.Hour), (-d).Round(time.Hour), (-d).Abs(), time.Duration(-1<<63).Abs())
	fmt.Println(d/time.Minute, d%time.Minute, d*2, d/3, -d)
	fmt.Println(time.Duration(1.5*float64(time.Second)), time.Second*time.Duration(2))

	inputs := []string{"300ms", "-1.5h", "2h45m", "1h2m3s4ms5us6ns", "1.000000001s", ".5s", "1µs", "1us", "0", "+5m",
		"", "1", "1d", "--1s", "1.s", "9223372036854775807ns", "9223372036854775808ns", "3000000h"}
	for _, in := range inputs {
		v, err := time.ParseDuration(in)
		fmt.Printf("%q -> %v %v\n", in, v, err)
	}

	// Comparison and sorting of durations.
	a, b := 90*time.Second, time.Minute+30*time.Second
	fmt.Println(a == b, a < 2*time.Minute, max(a, 2*b), min(a, time.Second))
	var total time.Duration
	for i := 1; i <= 10; i++ {
		total += time.Duration(i) * time.Millisecond
	}
	fmt.Println(total, total.Milliseconds())

	// Month and Weekday Strings need no clock.
	fmt.Println(time.March, time.Saturday, time.Month(13), time.Weekday(8))
	t := time.Date(2024, 2, 29, 13, 4, 5, 6, time.UTC)
	fmt.Println(t.Add(36*time.Hour).Format(time.RFC3339Nano), t.Sub(t.Add(-d)), t.AddDate(1, 0, 0).Format("2006-01-02"))
}
