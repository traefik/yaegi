// Native boundary of image/draw: program images given to Draw, DrawMask
// and Op.Draw (docs/BOUNDARY.md).
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"strings"
)

var cur string

// probe runs one probe: its lines start with its id, and its panic is printed.
func probe(id string, f func()) {
	cur = id
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("%s: panic: %T: %v\n", id, r, r)
		}
	}()
	f()
}

// out prints one line of the current probe.
func out(format string, args ...any) { fmt.Printf(cur+": "+format+"\n", args...) }

const size = 3

// calls records the methods called on the program images.
var calls = map[string]int{}

func callString() string {
	var keys []string
	for k := range calls {
		keys = append(keys, k)
	}
	// small insertion sort: no import of sort for three keys
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	var sb strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&sb, "%s=%d ", k, calls[k])
	}
	clear(calls)
	return strings.TrimSpace(sb.String())
}

// rgba64Img is a draw.RGBA64Image (pointer receivers).
type rgba64Img struct{ pix [size * size]color.RGBA64 }

func (m *rgba64Img) ColorModel() color.Model { return color.RGBA64Model }
func (m *rgba64Img) Bounds() image.Rectangle { return image.Rect(0, 0, size, size) }
func (m *rgba64Img) At(x, y int) color.Color { calls["At"]++; return m.pix[y*size+x] }
func (m *rgba64Img) Set(x, y int, c color.Color) {
	calls["Set"]++
	m.pix[y*size+x] = color.RGBA64Model.Convert(c).(color.RGBA64)
}
func (m *rgba64Img) RGBA64At(x, y int) color.RGBA64 { calls["RGBA64At"]++; return m.pix[y*size+x] }
func (m *rgba64Img) SetRGBA64(x, y int, c color.RGBA64) {
	calls["SetRGBA64"]++
	m.pix[y*size+x] = c
}

// genImg is a generic draw.RGBA64Image (value receivers over a slice).
type genImg[T any] struct {
	pix []color.RGBA64
	tag T
}

func (m genImg[T]) ColorModel() color.Model { return color.RGBA64Model }
func (m genImg[T]) Bounds() image.Rectangle { return image.Rect(0, 0, size, size) }
func (m genImg[T]) At(x, y int) color.Color { calls["gAt"]++; return m.pix[y*size+x] }
func (m genImg[T]) Set(x, y int, c color.Color) {
	calls["gSet"]++
	m.pix[y*size+x] = color.RGBA64Model.Convert(c).(color.RGBA64)
}
func (m genImg[T]) RGBA64At(x, y int) color.RGBA64 { calls["gRGBA64At"]++; return m.pix[y*size+x] }
func (m genImg[T]) SetRGBA64(x, y int, c color.RGBA64) {
	calls["gSetRGBA64"]++
	m.pix[y*size+x] = c
}

// plainImg is a draw.Image without RGBA64At and SetRGBA64.
type plainImg struct{ pix [size * size]color.RGBA64 }

func (m *plainImg) ColorModel() color.Model { return color.RGBA64Model }
func (m *plainImg) Bounds() image.Rectangle { return image.Rect(0, 0, size, size) }
func (m *plainImg) At(x, y int) color.Color { calls["pAt"]++; return m.pix[y*size+x] }
func (m *plainImg) Set(x, y int, c color.Color) {
	calls["pSet"]++
	m.pix[y*size+x] = color.RGBA64Model.Convert(c).(color.RGBA64)
}

// srcImg is an image.RGBA64Image that is not a draw.Image (value receivers).
type srcImg struct{ a uint16 }

func (srcImg) ColorModel() color.Model { return color.RGBA64Model }
func (srcImg) Bounds() image.Rectangle { return image.Rect(0, 0, size, size) }
func (m srcImg) At(x, y int) color.Color {
	calls["sAt"]++
	return color.RGBA64{uint16(x * 0x1000), uint16(y * 0x1000), 0, m.a}
}
func (m srcImg) RGBA64At(x, y int) color.RGBA64 {
	calls["sRGBA64At"]++
	return color.RGBA64{uint16(x * 0x1000), uint16(y * 0x1000), 0, m.a}
}

// atOnlyImg is an image.Image without RGBA64At.
type atOnlyImg struct{}

func (atOnlyImg) ColorModel() color.Model { return color.RGBA64Model }
func (atOnlyImg) Bounds() image.Rectangle { return image.Rect(0, 0, size, size) }
func (atOnlyImg) At(x, y int) color.Color {
	calls["oAt"]++
	return color.RGBA64{0x2000, 0x3000, uint16(x * 0x100), 0xffff}
}

// sliceImg is a draw.Image of an uncomparable type (value receivers):
// image/draw compares dst == src.
type sliceImg struct{ pix []color.Gray }

func (m sliceImg) ColorModel() color.Model { return color.GrayModel }
func (m sliceImg) Bounds() image.Rectangle { return image.Rect(0, 0, size, size) }
func (m sliceImg) At(x, y int) color.Color { return m.pix[y*size+x] }
func (m sliceImg) Set(x, y int, c color.Color) {
	m.pix[y*size+x] = color.GrayModel.Convert(c).(color.Gray)
}

func show(m image.Image) string {
	var sb strings.Builder
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			r, g, b, a := m.At(x, y).RGBA()
			fmt.Fprintf(&sb, "%x/%x/%x/%x ", r, g, b, a)
		}
	}
	return strings.TrimSpace(sb.String())
}

func main() {
	// row: image/draw Draw,DrawMask,Op.Draw assert x.(image/draw.RGBA64Image)
	// row: image/draw Draw,DrawMask,Op.Draw assert x.(image.RGBA64Image)
	probe("Draw/RGBA64Image", func() {
		srcs := []image.Image{srcImg{0xffff}, srcImg{0x8000}, atOnlyImg{}, image.NewUniform(color.RGBA64{1, 2, 3, 0xffff})}
		for i, src := range srcs {
			for _, op := range []draw.Op{draw.Src, draw.Over} {
				dsts := []draw.Image{&rgba64Img{}, genImg[int]{make([]color.RGBA64, size*size), 1}, &plainImg{}}
				for j, dst := range dsts {
					draw.Draw(dst, dst.Bounds(), src, image.Point{}, op)
					c := callString()
					out("Draw src %d op %d dst %d: %s | %s", i, op, j, c, show(dst))
					clear(calls)
				}
			}
		}
		dst := &rgba64Img{}
		draw.Src.Draw(dst, image.Rect(1, 1, 3, 3), srcImg{0xffff}, image.Point{})
		out("Op.Draw: %s | %s", callString(), show(dst))
		clear(calls)
		g := genImg[string]{make([]color.RGBA64, size*size), "m"}
		draw.DrawMask(g, g.Bounds(), atOnlyImg{}, image.Point{}, srcImg{0x4000}, image.Point{}, draw.Over)
		out("DrawMask mask: %s | %s", callString(), show(g))
		clear(calls)
		r := image.NewRGBA(image.Rect(0, 0, size, size))
		draw.DrawMask(r, r.Bounds(), srcImg{0xffff}, image.Point{}, image.NewAlpha(r.Bounds()), image.Point{}, draw.Over)
		out("DrawMask RGBA alpha mask: %s | %s", callString(), show(r))
		clear(calls)
		draw.Draw(r, r.Bounds(), srcImg{0xffff}, image.Point{}, draw.Src)
		out("Draw RGBA dst: %s | %s", callString(), show(r))
		clear(calls)
	})
	// row: image/draw Draw,DrawMask,Op.Draw panic == of two interfaces: comparing uncomparable type
	probe("Draw/uncomparable", func() {
		m := sliceImg{make([]color.Gray, size*size)}
		for _, f := range []struct {
			name string
			do   func()
		}{
			{"Draw other src", func() { draw.Draw(m, m.Bounds(), atOnlyImg{}, image.Point{}, draw.Src) }},
			{"Draw RGBA dst", func() {
				r := image.NewRGBA(image.Rect(0, 0, size, size))
				draw.Draw(r, r.Bounds(), m, image.Point{}, draw.Over)
				out("RGBA dst: %s", show(r))
			}},
			{"Draw same", func() { draw.Draw(m, m.Bounds(), m, image.Point{}, draw.Src) }},
			{"DrawMask same", func() { draw.DrawMask(m, m.Bounds(), m, image.Point{}, nil, image.Point{}, draw.Over) }},
			{"Op.Draw same", func() { draw.Over.Draw(m, m.Bounds(), m, image.Point{}) }},
		} {
			func() {
				defer func() {
					if r := recover(); r != nil {
						out("%s: panic: %T: %v", f.name, r, r)
					}
				}()
				f.do()
				out("%s: ok %s", f.name, show(m))
			}()
		}
	})
}
