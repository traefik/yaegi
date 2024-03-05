//go:build go1.21 && !go1.22
// +build go1.21,!go1.22

package generic

import _ "embed"

//go:embed go1_21_cmp.go.txt
var cmpSource string

//go:embed go1_21_maps.go.txt
var mapsSource string

//go:embed go1_21_slices.go.txt
var slicesSource string

/*
//go:embed go1_21_sync.go.txt
var syncSource string

//go:embed go1_21_sync_atomic.go.txt
var syncAtomicSource string
*/

// Sources contains the list of generic packages source strings.
var Sources = [...]string{
	cmpSource,
	mapsSource,
	slicesSource,
	// FIXME(marc): support the following.
	// syncAtomicSource,
	// syncSource,
}
