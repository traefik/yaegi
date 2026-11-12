package p6

import (
	"encoding/json"
	"net/netip"
)

type Slice[T any] struct {
	x []T
}

type IPPrefixSlice struct {
	x Slice[netip.Prefix]
}

func (v Slice[T]) MarshalJSON() ([]byte, error) { return json.Marshal(v.x) }

// MarshalJSON implements json.Marshaler.
func (v IPPrefixSlice) MarshalJSON() ([]byte, error) {
	return v.x.MarshalJSON()
}
