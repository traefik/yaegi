//go:build go1.22
// +build go1.22

package generic

import _ "embed"

//go:embed go1_22_cmp_cmp.go.txt
var cmpSource string

//go:embed go1_22_maps_maps.go.txt
var mapsSource string

//go:embed go1_22_slices_slices.go.txt
var slicesSource string

/*
//go:embed go1_22_slices_sort.go.txt
var slicesSource1 string

//go:embed go1_22_slices_zsortanyfunc.go.txt
var slicesSource2 string

//go:embed go1_22_sync_oncefunc.go.txt
var syncSource string

//go:embed go1_22_sync_atomic_type.go.txt
var syncAtomicSource string
*/

// Sources contains the list of generic packages source strings.
var Sources = [...]string{
	cmpSource,
	mapsSource,
	slicesSource,
	// FIXME(marc): support the following.
	// slicesSource1,
	// slicesSource2,
	// syncAtomicSource,
	// syncSource,
}
