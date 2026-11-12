// time with fixed dates in UTC and fixed zones: Date normalization, Add,
// Sub, AddDate, Format and Parse with many layouts, Truncate and Round,
// Weekday, YearDay, ISOWeek, Duration String and Round, and time values
// inside program structs marshaled with json and compared.
package main

import (
	"encoding/json"
	"fmt"
	"time"
)

type Event struct {
	Name  string        `json:"name"`
	At    time.Time     `json:"at"`
	Dur   time.Duration `json:"dur"`
	Until *time.Time    `json:"until,omitempty"`
}

func (e Event) End() time.Time { return e.At.Add(e.Dur) }

type Deadline time.Time

func (d Deadline) String() string { return "deadline " + time.Time(d).Format(time.DateOnly) }

func main() {
	base := time.Date(2024, time.February, 28, 23, 59, 59, 999999999, time.UTC)
	fmt.Println(base, base.Unix(), base.UnixMilli(), base.Nanosecond())
	fmt.Println(time.Date(2023, 13, 32, 25, 61, 61, 0, time.UTC), time.Date(2024, 0, 0, 0, 0, 0, 0, time.UTC))
	t := base.Add(time.Nanosecond)
	fmt.Println(t, t.Weekday(), t.YearDay(), t.Month(), int(t.Month()))
	y, w := t.ISOWeek()
	fmt.Println(y, w)
	for _, d := range []time.Duration{0, 1, 999, time.Microsecond + 1, 1500 * time.Millisecond, -90 * time.Minute, 36*time.Hour + 5*time.Second, 1<<63 - 1} {
		fmt.Println(d, d.Hours(), d.Minutes(), d.Seconds(), d.Milliseconds(), d.Round(time.Second), d.Truncate(time.Minute), d.Abs())
	}
	for _, ad := range [][3]int{{0, 1, 0}, {1, 0, 1}, {0, 0, -60}, {-4, 0, 0}, {0, 12, 0}} {
		fmt.Println(ad, base.AddDate(ad[0], ad[1], ad[2]))
	}
	jan31 := time.Date(2023, 1, 31, 12, 0, 0, 0, time.UTC)
	fmt.Println(jan31.AddDate(0, 1, 0), jan31.Sub(base), base.Sub(jan31).Hours(), jan31.Before(base), jan31.After(base), jan31.Equal(jan31.In(time.FixedZone("X", 3600))))
	layouts := []string{time.RFC3339, time.RFC3339Nano, time.RFC1123, time.RFC1123Z, time.RFC822, time.ANSIC, time.Kitchen, time.Stamp, time.StampMicro, time.DateTime, time.DateOnly, time.TimeOnly,
		"2006-01-02 15:04:05.000 MST", "Mon Jan _2 3:04PM", "02/01/06 .999999", "2006 002 __2", "Z07:00 -0700 -07", "January 2, 2006"}
	zones := []*time.Location{time.UTC, time.FixedZone("CET", 3600), time.FixedZone("", -5*3600-1800)}
	for _, z := range zones {
		tz := time.Date(2009, 11, 10, 7, 4, 5, 123456000, z)
		for _, l := range layouts {
			fmt.Printf("%-30q %s\n", l, tz.Format(l))
		}
		fmt.Println(tz.UTC(), tz.Location(), tz.Unix())
		name, off := tz.Zone()
		fmt.Println(name, off)
	}
	parses := []struct{ layout, value string }{
		{time.RFC3339, "2021-06-01T10:20:30Z"},
		{time.RFC3339, "2021-06-01T10:20:30.5+02:00"},
		{time.RFC3339, "2021-06-01 10:20:30Z"},
		{time.DateOnly, "2021-02-29"},
		{time.DateOnly, "2020-02-29"},
		{"Jan 2 2006 15:04", "Mar 9 2022 7:05"},
		{"Jan 2 2006 15:04", "Mar 9 2022 17:05"},
		{time.Kitchen, "3:04PM"},
		{"2006-01-02 15:04:05.000", "2022-01-02 03:04:05.123"},
		{"02 Jan 06 15:04 -0700", "05 Nov 21 08:00 +0530"},
		{time.RFC1123, "Mon, 02 Jan 2006 15:04:05 UTC"},
		{"2006", "abcd"},
	}
	for _, p := range parses {
		pt, err := time.Parse(p.layout, p.value)
		fmt.Println(p.value, "=>", pt, err)
	}
	pt, err := time.ParseInLocation(time.DateTime, "2022-07-01 12:00:00", time.FixedZone("Z5", 5*3600))
	fmt.Println(pt, pt.UTC(), err)
	for _, s := range []string{"1h2m3.5s", "-1.5h", "300ms", "1us", "1µs", "2h45m", "", "1x", "9999999999h"} {
		d, err := time.ParseDuration(s)
		fmt.Println(s, d, err)
	}
	tr := time.Date(2024, 3, 15, 13, 47, 31, 600000000, time.UTC)
	for _, d := range []time.Duration{time.Second, time.Minute, 15 * time.Minute, time.Hour, 24 * time.Hour, 7 * time.Millisecond} {
		fmt.Println(d, tr.Truncate(d), tr.Round(d))
	}
	// Times in program types.
	until := tr.Add(48 * time.Hour)
	evs := []Event{{"a", tr, 90 * time.Minute, nil}, {"b", base, time.Second, &until}}
	b, err := json.Marshal(evs)
	fmt.Println(string(b), err)
	var back []Event
	fmt.Println(json.Unmarshal(b, &back))
	for _, e := range back {
		fmt.Println(e.Name, e.At, e.Dur, e.End(), e.At.Equal(tr) || e.At.Equal(base), e.Until != nil)
	}
	var bad Event
	fmt.Println(json.Unmarshal([]byte(`{"at":"yesterday"}`), &bad))
	fmt.Println(Deadline(tr), fmt.Sprint(Deadline(tr)))
	tb, _ := tr.MarshalText()
	var tt time.Time
	fmt.Println(string(tb), tt.UnmarshalText(tb), tt.Equal(tr), tt.IsZero(), time.Time{}.IsZero(), time.Time{})
	bin, _ := tr.MarshalBinary()
	var tbin time.Time
	fmt.Println(len(bin), tbin.UnmarshalBinary(bin), tbin.Equal(tr))
	fmt.Println(time.Unix(1700000000, 5).UTC(), time.UnixMilli(-1).UTC(), time.Unix(0, 0).UTC().Add(-time.Nanosecond))
	fmt.Println(tr.Compare(base), base.Compare(tr), tr.Compare(tr), time.Duration(1234567).String(), time.Month(13), time.Weekday(8))
	fmt.Println(tr.AppendFormat([]byte("at: "), time.Stamp))
}
