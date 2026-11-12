// min and max of NaN and an infinity are NaN: any NaN argument wins (the
// builtins, unlike math.Max and math.Min, whose special case is the infinity).
package main

import (
	"fmt"
	"math"
)

func main() {
	nan, inf := math.NaN(), math.Inf(1)
	fmt.Println(max(nan, inf), min(nan, -inf))
	fmt.Println(max(inf, nan), min(-inf, nan))
	fmt.Println(min(nan, inf), max(nan, -inf))
	var f32 = float32(nan)
	fmt.Println(max(f32, float32(inf)), min(float32(-inf), f32))
}
