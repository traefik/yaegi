// strconv.ParseInt, ParseUint, ParseFloat, ParseBool, ParseComplex and Atoi
// with bases, bit sizes, prefixes, underscores, range and syntax errors
// (printed with %v and inspected as *strconv.NumError).
package main

import (
	"errors"
	"fmt"
	"strconv"
)

func show(v any, err error) {
	if err != nil {
		var ne *strconv.NumError
		if errors.As(err, &ne) {
			fmt.Printf("err=%v func=%s num=%q range=%v syntax=%v | ", err, ne.Func, ne.Num,
				errors.Is(err, strconv.ErrRange), errors.Is(err, strconv.ErrSyntax))
		} else {
			fmt.Printf("err=%v | ", err)
		}
	}
	fmt.Printf("%T %v\n", v, v)
}

func main() {
	ints := []string{"0", "-0", "+5", "127", "128", "-128", "-129", "255", "32767", "-32769", "2147483647", "2147483648",
		"9223372036854775807", "9223372036854775808", "-9223372036854775808", "-9223372036854775809",
		"0x1F", "0X1f", "0b101", "0o17", "017", "1_000", "_1", "1__0", "0x_1", "", "-", "12a", " 1", "1e3", "ff", "zz", "-0x80"}
	for _, s := range ints {
		for _, bits := range []int{0, 8, 16, 32, 64} {
			v, err := strconv.ParseInt(s, 0, bits)
			fmt.Print(s, " base0 bits", bits, ": ")
			show(v, err)
		}
		v, err := strconv.ParseInt(s, 10, 64)
		fmt.Print(s, " base10: ")
		show(v, err)
		v, err = strconv.ParseInt(s, 16, 64)
		fmt.Print(s, " base16: ")
		show(v, err)
		v, err = strconv.ParseInt(s, 36, 32)
		fmt.Print(s, " base36: ")
		show(v, err)
	}
	for _, s := range []string{"0", "255", "256", "65535", "65536", "4294967295", "4294967296", "18446744073709551615",
		"18446744073709551616", "-1", "+1", "0xFFFFFFFFFFFFFFFF", "0b1111", "1_2_3"} {
		for _, bits := range []int{0, 8, 16, 32, 64} {
			v, err := strconv.ParseUint(s, 0, bits)
			fmt.Print(s, " u", bits, ": ")
			show(v, err)
		}
	}
	_, err := strconv.ParseInt("1", 1, 64)
	show(0, err)
	_, err = strconv.ParseInt("1", 37, 64)
	show(0, err)
	_, err = strconv.ParseInt("1", 10, 65)
	show(0, err)
	_, err = strconv.ParseUint("1", 10, -1)
	show(0, err)
	for _, s := range []string{"12", "-12", "2147483648", "x", "", "99999999999999999999"} {
		v, err := strconv.Atoi(s)
		fmt.Print(s, " atoi: ")
		if err != nil {
			fmt.Println("err", err)
		} else {
			fmt.Println(int64(v))
		}
	}
	floats := []string{"0", "-0", "1.5", "-2.25e3", ".5", "5.", "1e308", "1e309", "-1e309", "4.9e-324", "1e-400", "inf", "-Inf",
		"+INF", "infinity", "NaN", "nan", "0x1p-2", "0x1.8p1", "0X.8P+0", "1_000.5", "1e", "e1", "", "1.2.3", "3.4028235e38",
		"3.5e38", "1.401298464324817e-45", "0.1", "100000000000000016777215", "0x1p", "1__0"}
	for _, s := range floats {
		v, err := strconv.ParseFloat(s, 64)
		fmt.Print(s, " f64: ")
		show(v, err)
		v, err = strconv.ParseFloat(s, 32)
		fmt.Print(s, " f32: ")
		show(v, err)
	}
	for _, s := range []string{"1", "t", "T", "TRUE", "true", "True", "0", "f", "F", "FALSE", "false", "False", "yes", ""} {
		v, err := strconv.ParseBool(s)
		fmt.Print(s, " bool: ")
		show(v, err)
	}
	for _, s := range []string{"1+2i", "(3-4i)", "-1.5i", "2", "NaN+Infi", "1+2j", "1e400+1i", "i"} {
		v, err := strconv.ParseComplex(s, 128)
		fmt.Print(s, " c128: ")
		show(v, err)
		v, err = strconv.ParseComplex(s, 64)
		fmt.Print(s, " c64: ")
		show(v, err)
	}
}
