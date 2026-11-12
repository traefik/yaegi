// text/tabwriter: padding, minwidth, padchar, flags (AlignRight, Debug,
// FilterHTML, StripEscape, DiscardEmptyColumns, TabIndent), escapes,
// multi-byte cells, and Flush in the middle of the input.
package main

import (
	"fmt"
	"strings"
	"text/tabwriter"
)

func main() {
	text := "name\tage\tcity\n" +
		"alice\t30\tParis\n" +
		"bob\t4\tSão Paulo\n" +
		"\t\t\n" +
		"émile\t1234567\t北京\n" +
		"no tabs here\n" +
		"x\ty\n"
	type cfg struct {
		min, tab, pad int
		ch            byte
		flags         uint
	}
	cfgs := []cfg{
		{0, 8, 1, ' ', 0},
		{5, 0, 2, '.', 0},
		{0, 4, 1, ' ', tabwriter.AlignRight},
		{0, 8, 1, ' ', tabwriter.Debug},
		{0, 8, 1, '\t', tabwriter.TabIndent},
		{0, 8, 1, '-', tabwriter.DiscardEmptyColumns},
		{0, 8, 1, ' ', tabwriter.AlignRight | tabwriter.Debug},
	}
	for i, c := range cfgs {
		var sb strings.Builder
		w := tabwriter.NewWriter(&sb, c.min, c.tab, c.pad, c.ch, c.flags)
		fmt.Fprint(w, text)
		w.Flush()
		fmt.Printf("== %d\n%s", i, sb.String())
	}
	var sb strings.Builder
	w := tabwriter.NewWriter(&sb, 0, 8, 1, ' ', tabwriter.FilterHTML|tabwriter.StripEscape)
	fmt.Fprint(w, "<b>bold</b>\t&amp;\tx\n\xffesc\taped\xff\tlong text\ty\na\tb\tc\n")
	w.Flush()
	fmt.Printf("== html\n%s", sb.String())
	sb.Reset()
	w = tabwriter.NewWriter(&sb, 0, 8, 1, ' ', 0)
	fmt.Fprint(w, "a\tb\n")
	w.Flush()
	fmt.Fprint(w, "longer cell\tb\n")
	fmt.Fprint(w, "c\vd\fe\tf\n")
	w.Flush()
	fmt.Printf("== flush\n%s", sb.String())
	sb.Reset()
	w = tabwriter.NewWriter(&sb, 0, 8, 1, ' ', 0)
	fmt.Fprint(w, "\xff\tescaped\xff\tz\nq\tr")
	w.Flush()
	fmt.Printf("== esc\n%q\n", sb.String())
	fmt.Println(tabwriter.Escape)
}
