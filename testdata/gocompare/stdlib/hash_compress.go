// hash/crc32, crc64, adler32, fnv, maphash with a fixed seed is left out;
// crypto/sha256, sha1, sha512, md5 and hmac of fixed inputs written by
// program Writers; compress/gzip, zlib, flate round trips in memory and
// archive/zip and archive/tar round trips (names, contents, sizes).
package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash"
	"hash/adler32"
	"hash/crc32"
	"hash/crc64"
	"hash/fnv"
	"io"
	"strings"
	"time"
)

type Record struct {
	ID   int
	Body string
}

// WriteTo writes the record into any hash or writer.
func (r Record) WriteTo(w io.Writer) (int64, error) {
	n, err := fmt.Fprintf(w, "%d:%s|", r.ID, r.Body)
	return int64(n), err
}

func digest(h hash.Hash, recs []Record) string {
	for _, r := range recs {
		r.WriteTo(h)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func main() {
	recs := []Record{{1, "alpha"}, {2, "beta"}, {3, strings.Repeat("gamma", 50)}}
	hashes := []struct {
		name string
		h    hash.Hash
	}{
		{"md5", md5.New()}, {"sha1", sha1.New()}, {"sha256", sha256.New()}, {"sha224", sha256.New224()},
		{"sha512", sha512.New()}, {"sha512/256", sha512.New512_256()}, {"hmac", hmac.New(sha256.New, []byte("key"))},
		{"crc32", crc32.NewIEEE()}, {"crc32c", crc32.New(crc32.MakeTable(crc32.Castagnoli))}, {"crc64", crc64.New(crc64.MakeTable(crc64.ECMA))},
		{"adler32", adler32.New()}, {"fnv32", fnv.New32()}, {"fnv32a", fnv.New32a()}, {"fnv64a", fnv.New64a()}, {"fnv128", fnv.New128()},
	}
	for _, h := range hashes {
		fmt.Println(h.name, h.h.Size(), h.h.BlockSize(), digest(h.h, recs))
		h.h.Reset()
		fmt.Println("  empty:", hex.EncodeToString(h.h.Sum([]byte{0xAA})))
	}
	fmt.Printf("%x\n%x\n%x\n", md5.Sum([]byte("abc")), sha256.Sum256([]byte("abc")), sha1.Sum(nil))
	fmt.Println(crc32.ChecksumIEEE([]byte("hello")), adler32.Checksum([]byte("hello")), crc64.Checksum([]byte("hello"), crc64.MakeTable(crc64.ISO)))
	f := fnv.New64()
	f.Write([]byte("x"))
	fmt.Println(f.Sum64())
	mac := hmac.New(sha256.New, []byte("k"))
	mac.Write([]byte("msg"))
	sum := mac.Sum(nil)
	mac2 := hmac.New(sha256.New, []byte("k"))
	mac2.Write([]byte("msg"))
	fmt.Println(hmac.Equal(sum, mac2.Sum(nil)), hmac.Equal(sum, sum[:5]))

	// Compression round trips: only the decompressed data is printed.
	var payload bytes.Buffer
	for i := 0; i < 200; i++ {
		recs[i%3].WriteTo(&payload)
	}
	data := payload.Bytes()
	var gz bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&gz, gzip.BestCompression)
	zw.Name = "records.txt"
	zw.Comment = "program records"
	zw.ModTime = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	n, err := zw.Write(data)
	fmt.Println(n, err, zw.Close(), gz.Len() < len(data))
	zr, err := gzip.NewReader(&gz)
	fmt.Println(err, zr.Name, zr.Comment, zr.ModTime.UTC())
	out, err := io.ReadAll(zr)
	fmt.Println(len(out), err, bytes.Equal(out, data), string(out[:30]))
	_, err = gzip.NewReader(strings.NewReader("not gzip data"))
	fmt.Println(err)
	var zb bytes.Buffer
	zlw := zlib.NewWriter(&zb)
	zlw.Write(data)
	zlw.Close()
	zlr, err := zlib.NewReader(&zb)
	out, _ = io.ReadAll(zlr)
	fmt.Println(err, len(out), bytes.Equal(out, data))
	var fb bytes.Buffer
	fw, _ := flate.NewWriter(&fb, flate.BestSpeed)
	fw.Write(data[:100])
	fw.Flush()
	fw.Write(data[100:])
	fw.Close()
	out, err = io.ReadAll(flate.NewReader(&fb))
	fmt.Println(err, len(out), bytes.Equal(out, data))
	_, err = io.ReadAll(flate.NewReader(strings.NewReader("\xff\xff\xff")))
	fmt.Println(err)

	// archive/zip.
	var zbuf bytes.Buffer
	zwr := zip.NewWriter(&zbuf)
	files := []struct{ name, body string }{{"a.txt", "hello"}, {"dir/b.txt", strings.Repeat("b", 1000)}, {"dir/", ""}, {"c.bin", string(data[:50])}}
	for i, fl := range files {
		hdr := &zip.FileHeader{Name: fl.name, Method: zip.Deflate, Modified: time.Date(2021, 3, 4, 5, 6, 8, 0, time.UTC)}
		if i == 0 {
			hdr.Method = zip.Store
		}
		fw, err := zwr.CreateHeader(hdr)
		if err != nil {
			fmt.Println(err)
			continue
		}
		if fl.body != "" {
			fw.Write([]byte(fl.body))
		}
	}
	zwr.SetComment("zip comment")
	fmt.Println(zwr.Close())
	zrd, err := zip.NewReader(bytes.NewReader(zbuf.Bytes()), int64(zbuf.Len()))
	fmt.Println(err, len(zrd.File), zrd.Comment)
	for _, zf := range zrd.File {
		rc, err := zf.Open()
		if err != nil {
			fmt.Println(zf.Name, err)
			continue
		}
		body, _ := io.ReadAll(rc)
		rc.Close()
		fmt.Println(zf.Name, zf.Method, zf.UncompressedSize64, zf.FileInfo().IsDir(), zf.Modified.UTC(), len(body), crc32.ChecksumIEEE(body) == zf.CRC32)
	}
	_, err = zip.NewReader(strings.NewReader("nope"), 4)
	fmt.Println(err)

	// archive/tar.
	var tb bytes.Buffer
	tw := tar.NewWriter(&tb)
	for _, fl := range files[:2] {
		tw.WriteHeader(&tar.Header{Name: fl.name, Mode: 0o644, Size: int64(len(fl.body)), ModTime: time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC), Format: tar.FormatPAX})
		tw.Write([]byte(fl.body))
	}
	fmt.Println(tw.Close())
	tr := tar.NewReader(&tb)
	for {
		h, err := tr.Next()
		if err != nil {
			fmt.Println("tar end:", err)
			break
		}
		body, _ := io.ReadAll(tr)
		fmt.Println(h.Name, h.Size, h.Mode, h.ModTime.UTC(), len(body), string(body[:5]))
	}
}
