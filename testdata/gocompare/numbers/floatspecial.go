// float64 and float32 special values: NaN comparisons, signed zero, infinity
// arithmetic, and how fmt prints them; maps and switches with NaN and -0.
package main

import (
	"fmt"
	"math"
	"sort"
)

func main() {
	nan := math.NaN()
	inf := math.Inf(1)
	ninf := math.Inf(-1)
	var zero float64
	negz := math.Copysign(0, -1)
	fmt.Println(nan, inf, ninf, zero, negz)
	fmt.Println(nan == nan, nan != nan, nan < 1, nan > 1, nan <= nan, nan >= nan)
	fmt.Println(inf == inf, inf > math.MaxFloat64, ninf < -math.MaxFloat64, inf+ninf, inf-inf, inf*0, inf/inf)
	fmt.Println(zero == negz, 1/zero, 1/negz, math.Signbit(negz), math.Signbit(zero), -zero, 0*-1.0)
	fmt.Println(negz+0, negz-0, negz*negz, negz+negz, zero-zero, -zero-zero)
	x := 1.0
	fmt.Println(x/zero, -x/zero, zero/zero, math.IsNaN(zero/zero))
	fmt.Println(math.IsInf(inf, 0), math.IsInf(inf, 1), math.IsInf(inf, -1), math.IsInf(ninf, -1), math.IsInf(nan, 0))
	mf := math.MaxFloat64
	fmt.Println(mf*2, -mf*2, math.SmallestNonzeroFloat64/2, math.SmallestNonzeroFloat64)
	// Same with float32.
	n32 := float32(nan)
	i32 := float32(inf)
	z32 := float32(0)
	fmt.Println(n32, i32, -i32, n32 == n32, i32 > math.MaxFloat32, z32 == -z32, float32(negz))
	fmt.Println(math.Signbit(float64(float32(negz))), 1/float32(negz))
	// Map keys: NaN keys are all different, -0 and 0 are the same key.
	m := map[float64]int{}
	m[nan] = 1
	m[nan] = 2
	m[zero] = 3
	m[negz] = 4
	m[inf] = 5
	fmt.Println(len(m), m[zero], m[nan], m[inf])
	_, ok := m[nan]
	fmt.Println(ok)
	sum := 0
	for k, v := range m {
		if k != k {
			sum += v
		}
	}
	fmt.Println(sum)
	delete(m, nan)
	fmt.Println(len(m))
	clear(m)
	fmt.Println(len(m))
	// Switch with NaN never matches.
	switch nan {
	case nan:
		fmt.Println("matched nan")
	default:
		fmt.Println("default for nan")
	}
	switch negz {
	case 0:
		fmt.Println("negz is 0")
	}
	// Comparison of arrays and structs with NaN.
	type P struct{ A float64 }
	fmt.Println(P{nan} == P{nan}, [2]float64{0, 1} == [2]float64{negz, 1}, [1]float64{nan} != [1]float64{nan})
	// Interface comparison with NaN.
	var ia, ib any = nan, nan
	fmt.Println(ia == ib, ia == ia)
	// Sorting with NaN: sort.Float64s puts NaN first.
	s := []float64{3, nan, -1, inf, negz, ninf, 0, nan, 2}
	sort.Float64s(s)
	fmt.Println(s)
	// min and max with NaN and signed zeros.
	fmt.Println(min(1, nan), max(nan, 1), min(zero, negz), max(negz, zero), min(inf, ninf), max(1.0, inf))
	fmt.Println(math.Signbit(min(zero, negz)), math.Signbit(max(negz, zero)), math.Signbit(min(negz, zero)))
	// fmt with special values.
	fmt.Printf("%f %e %g %v %5.2f %+f %08.3f %-8f|\n", nan, inf, ninf, negz, nan, inf, ninf, nan)
	fmt.Printf("%x %X %b %+.3v % f\n", inf, negz, nan, negz, inf)
	fmt.Printf("%f %.0f %g %e\n", negz, negz, float32(negz), negz)
	// Comparisons of NaN in a loop.
	cnt := 0
	for _, v := range []float64{nan, 1, nan, 2} {
		if v == v {
			cnt++
		}
	}
	fmt.Println(cnt)
	// NaN payloads are not printed: only check that NaN bits classify as NaN.
	fmt.Println(math.IsNaN(math.Float64frombits(0x7FF8000000000001)), math.IsNaN(float64(math.Float32frombits(0x7FC00001))))
	fmt.Println(math.Float64bits(inf), math.Float64bits(negz), math.Float32bits(float32(ninf)))
}
