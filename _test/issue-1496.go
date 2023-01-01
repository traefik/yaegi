package main

func ByteEqualsNil() bool {
	return []byte{} == nil
}

func NilEqualsByte() bool {
	return nil == []byte{}
}

func main() {
	a := ByteEqualsNil()
	b := NilEqualsByte()

	println(a == false, b == false, a == b)
}

// Output:
// true true true
