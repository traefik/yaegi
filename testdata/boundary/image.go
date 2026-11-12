// Native boundary of image, image/png, image/gif and image/jpeg: program
// readers, writers and images given to these packages (docs/BOUNDARY.md).
package main

import (
	"bytes"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
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

// peekReader has Peek (pointer receiver): it is the reader of image.
type peekReader struct {
	r     *bytes.Reader
	peeks int
}

func (p *peekReader) Read(b []byte) (int, error) { return p.r.Read(b) }
func (p *peekReader) Peek(n int) ([]byte, error) {
	p.peeks++
	rest := make([]byte, p.r.Len())
	pos, _ := p.r.Seek(0, io.SeekCurrent)
	m, _ := p.r.Read(rest)
	p.r.Seek(pos, io.SeekStart)
	if m < n {
		return rest[:m], io.EOF
	}
	return rest[:n], nil
}

// genPeekReader is a generic reader with Peek.
type genPeekReader[T any] struct{ p *peekReader }

func (g genPeekReader[T]) Read(b []byte) (int, error) { return g.p.Read(b) }
func (g genPeekReader[T]) Peek(n int) ([]byte, error) { return g.p.Peek(n) }

// byteReader has ReadByte (value receiver, shared state): it is the
// reader of image/gif.
type byteReader struct {
	r *bytes.Reader
	n *int
}

func (b byteReader) Read(p []byte) (int, error) { return b.r.Read(p) }
func (b byteReader) ReadByte() (byte, error)    { *b.n++; return b.r.ReadByte() }

// onlyReader has only Read.
type onlyReader struct{ r *bytes.Reader }

func (o *onlyReader) Read(p []byte) (int, error) { return o.r.Read(p) }

// flushWriter has Flush and WriteByte (pointer receivers): it is the
// writer of image/gif and image/jpeg.
type flushWriter struct {
	b       bytes.Buffer
	flushes int
	bytes   int
}

func (w *flushWriter) Write(p []byte) (int, error) { return w.b.Write(p) }
func (w *flushWriter) WriteByte(c byte) error      { w.bytes++; return w.b.WriteByte(c) }
func (w *flushWriter) Flush() error                { w.flushes++; return nil }

// genFlushWriter is a generic writer with Flush and WriteByte.
type genFlushWriter[T any] struct{ w *flushWriter }

func (g genFlushWriter[T]) Write(p []byte) (int, error) { return g.w.Write(p) }
func (g genFlushWriter[T]) WriteByte(c byte) error      { return g.w.WriteByte(c) }
func (g genFlushWriter[T]) Flush() error                { return g.w.Flush() }

// flushOnly has Flush but no WriteByte.
type flushOnly struct {
	b       bytes.Buffer
	flushes int
}

func (w *flushOnly) Write(p []byte) (int, error) { return w.b.Write(p) }
func (w *flushOnly) Flush() error                { w.flushes++; return nil }

var pal2 = color.Palette{color.Gray{0}, color.Gray{255}}

func pal20() color.Palette {
	var p color.Palette
	for i := 0; i < 20; i++ {
		p = append(p, color.RGBA{uint8(i * 12), 0, uint8(255 - i*12), 255})
	}
	return p
}

// palImg is a PalettedImage of 2 colors (pointer receivers).
type palImg struct{ w, h int }

func (m *palImg) ColorModel() color.Model        { return pal2 }
func (m *palImg) Bounds() image.Rectangle        { return image.Rect(0, 0, m.w, m.h) }
func (m *palImg) At(x, y int) color.Color        { return pal2[m.ColorIndexAt(x, y)] }
func (m *palImg) ColorIndexAt(x, y int) uint8    { return uint8((x + y) % 2) }
func (m *palImg) String() string                 { return "palImg" }
func (m *palImg) RGBA64At(x, y int) color.RGBA64 { return color.RGBA64{} }

// genPalImg is a generic PalettedImage of 20 colors (value receivers).
type genPalImg[T any] struct {
	pal color.Palette
	tag T
}

func (m genPalImg[T]) ColorModel() color.Model     { return m.pal }
func (m genPalImg[T]) Bounds() image.Rectangle     { return image.Rect(0, 0, 5, 4) }
func (m genPalImg[T]) At(x, y int) color.Color     { return m.pal[m.ColorIndexAt(x, y)] }
func (m genPalImg[T]) ColorIndexAt(x, y int) uint8 { return uint8((x*4 + y) % 20) }

// noIndexImg has a color.Palette model but no ColorIndexAt.
type noIndexImg struct{}

func (noIndexImg) ColorModel() color.Model { return pal2 }
func (noIndexImg) Bounds() image.Rectangle { return image.Rect(0, 0, 3, 2) }
func (noIndexImg) At(x, y int) color.Color { return pal2[(x+y)%2] }

// opaqueImg has transparent pixels, but its Opaque method says true.
type opaqueImg struct{ a uint8 }

func (opaqueImg) ColorModel() color.Model   { return color.RGBAModel }
func (opaqueImg) Bounds() image.Rectangle   { return image.Rect(0, 0, 2, 2) }
func (m opaqueImg) At(x, y int) color.Color { return color.RGBA{m.a, 0, 0, m.a} }
func (opaqueImg) Opaque() bool              { return true }

// alphaImg has transparent pixels and no Opaque method.
type alphaImg struct{ a uint8 }

func (alphaImg) ColorModel() color.Model   { return color.RGBAModel }
func (alphaImg) Bounds() image.Rectangle   { return image.Rect(0, 0, 2, 2) }
func (m alphaImg) At(x, y int) color.Color { return color.RGBA{m.a, 0, 0, m.a} }

// solidImg is opaque, has no Opaque method and a 16-bit model.
type solidImg struct{}

func (solidImg) ColorModel() color.Model { return color.RGBA64Model }
func (solidImg) Bounds() image.Rectangle { return image.Rect(0, 0, 2, 1) }
func (solidImg) At(x, y int) color.Color { return color.RGBA64{0x1234, 0x5678, 0x9abc, 0xffff} }

func pngBytes() []byte {
	var b bytes.Buffer
	png.Encode(&b, image.NewGray(image.Rect(0, 0, 3, 2)))
	return b.Bytes()
}

func gifBytes() []byte {
	var b bytes.Buffer
	m := image.NewPaletted(image.Rect(0, 0, 4, 3), pal2)
	m.Pix[5] = 1
	gif.EncodeAll(&b, &gif.GIF{Image: []*image.Paletted{m, m}, Delay: []int{1, 2}})
	return b.Bytes()
}

// describe decodes a png and prints its type and first pixels.
func describe(data []byte) string {
	m, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return err.Error()
	}
	b := m.Bounds()
	s := fmt.Sprintf("%T %v", m, b)
	for x := b.Min.X; x < b.Max.X && x < 3; x++ {
		r, g, bl, a := m.At(x, b.Min.Y).RGBA()
		s += fmt.Sprintf(" (%x,%x,%x,%x)", r, g, bl, a)
	}
	return s
}

func main() {
	// row: image Decode,DecodeConfig assert x.(image.reader)
	probe("Decode/reader", func() {
		data := pngBytes()
		pr := &peekReader{r: bytes.NewReader(data)}
		m, f, err := image.Decode(pr)
		out("peekReader Decode: %T %s %v peeks=%d", m, f, err, pr.peeks)
		pr = &peekReader{r: bytes.NewReader(data)}
		c, f, err := image.DecodeConfig(pr)
		out("peekReader DecodeConfig: %d %d %s %v peeks=%d", c.Width, c.Height, f, err, pr.peeks)
		g := genPeekReader[int]{&peekReader{r: bytes.NewReader(data)}}
		m, f, err = image.Decode(g)
		out("generic Decode: %T %s %v peeks=%d", m, f, err, g.p.peeks)
		m, f, err = image.Decode(&onlyReader{bytes.NewReader(data)})
		out("onlyReader Decode: %T %s %v", m, f, err)
		c, f, err = image.DecodeConfig(&onlyReader{bytes.NewReader(data)})
		out("onlyReader DecodeConfig: %d %d %s %v", c.Width, c.Height, f, err)
		var r io.Reader = &peekReader{r: bytes.NewReader([]byte("not an image"))}
		_, f, err = image.Decode(r)
		out("iface Decode: %q %v peeks=%d", f, err, r.(*peekReader).peeks)
	})
	// row: image/png Encode,(*Encoder).Encode assert x.(image.PalettedImage)
	// row: image/png Encode,(*Encoder).Encode panic x.(image.PalettedImage) without ok: *runtime.TypeAssertionError
	// row: image/png Encode,(*Encoder).Encode assert x.(image/png.opaquer)
	probe("png/Encode", func() {
		ims := []image.Image{
			&palImg{5, 3}, genPalImg[string]{pal20(), "g"}, noIndexImg{},
			opaqueImg{0x80}, alphaImg{0x80}, alphaImg{0xff}, solidImg{},
		}
		for i, m := range ims {
			var b bytes.Buffer
			err := png.Encode(&b, m)
			out("Encode %d %T: %v crc=%08x %s", i, m, err, crc32.ChecksumIEEE(b.Bytes()), describe(b.Bytes()))
			b.Reset()
			enc := &png.Encoder{CompressionLevel: png.BestSpeed}
			err = enc.Encode(&b, m)
			out("Encoder %d: %v crc=%08x", i, err, crc32.ChecksumIEEE(b.Bytes()))
		}
	})
	// row: image/gif Decode,DecodeAll,DecodeConfig assert x.(image/gif.reader)
	probe("gif/Decode", func() {
		data := gifBytes()
		n := 0
		m, err := gif.Decode(byteReader{bytes.NewReader(data), &n})
		out("byteReader Decode: %v %v ReadByte=%v", m.Bounds(), err, n > 0)
		n = 0
		all, err := gif.DecodeAll(byteReader{bytes.NewReader(data), &n})
		out("byteReader DecodeAll: %d %v %v ReadByte=%v", len(all.Image), all.Delay, err, n > 0)
		n = 0
		c, err := gif.DecodeConfig(byteReader{bytes.NewReader(data), &n})
		out("byteReader DecodeConfig: %d %d %v ReadByte=%v", c.Width, c.Height, err, n > 0)
		m, err = gif.Decode(&onlyReader{bytes.NewReader(data)})
		out("onlyReader Decode: %v %v", m.Bounds(), err)
		all, err = gif.DecodeAll(&onlyReader{bytes.NewReader(data)})
		out("onlyReader DecodeAll: %d %v", len(all.Image), err)
		c, err = gif.DecodeConfig(&onlyReader{bytes.NewReader(data)})
		out("onlyReader DecodeConfig: %d %d %v", c.Width, c.Height, err)
	})
	// row: image/gif Encode,EncodeAll assert x.(image/gif.writer)
	probe("gif/Encode", func() {
		m := image.NewPaletted(image.Rect(0, 0, 4, 3), pal2)
		m.Pix[2] = 1
		fw := &flushWriter{}
		err := gif.Encode(fw, m, nil)
		out("flushWriter Encode: %v crc=%08x flushes=%d WriteByte=%v", err, crc32.ChecksumIEEE(fw.b.Bytes()), fw.flushes, fw.bytes > 0)
		gw := genFlushWriter[bool]{&flushWriter{}}
		err = gif.EncodeAll(gw, &gif.GIF{Image: []*image.Paletted{m}, Delay: []int{3}})
		out("generic EncodeAll: %v crc=%08x flushes=%d WriteByte=%v", err, crc32.ChecksumIEEE(gw.w.b.Bytes()), gw.w.flushes, gw.w.bytes > 0)
		fo := &flushOnly{}
		err = gif.Encode(fo, m, nil)
		out("flushOnly Encode: %v crc=%08x flushes=%d", err, crc32.ChecksumIEEE(fo.b.Bytes()), fo.flushes)
		fo = &flushOnly{}
		err = gif.EncodeAll(fo, &gif.GIF{Image: []*image.Paletted{m}, Delay: []int{3}})
		out("flushOnly EncodeAll: %v crc=%08x flushes=%d", err, crc32.ChecksumIEEE(fo.b.Bytes()), fo.flushes)
	})
	// row: image/jpeg Encode assert x.(image/jpeg.writer)
	probe("jpeg/Encode", func() {
		m := image.NewGray(image.Rect(0, 0, 8, 8))
		for i := range m.Pix {
			m.Pix[i] = uint8(i * 4)
		}
		fw := &flushWriter{}
		err := jpeg.Encode(fw, m, &jpeg.Options{Quality: 80})
		out("flushWriter: %v crc=%08x flushes=%d WriteByte=%v", err, crc32.ChecksumIEEE(fw.b.Bytes()), fw.flushes, fw.bytes > 0)
		gw := genFlushWriter[int]{&flushWriter{}}
		err = jpeg.Encode(gw, m, nil)
		out("generic: %v crc=%08x flushes=%d", err, crc32.ChecksumIEEE(gw.w.b.Bytes()), gw.w.flushes)
		fo := &flushOnly{}
		err = jpeg.Encode(fo, m, &jpeg.Options{Quality: 80})
		out("flushOnly: %v crc=%08x flushes=%d", err, crc32.ChecksumIEEE(fo.b.Bytes()), fo.flushes)
	})
}
