// //go:embed into an embed.FS (OPA embeds its capability files this way):
// files, directories (without the names that start with "." or "_", unless
// "all:"), hidden files named by a pattern, the order and modes of the
// entries, and io/fs over the file system. A string from a directory that
// holds one file.
package main

import (
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"sort"
)

//go:embed goembedfs/*.txt goembedfs/dir
//go:embed all:goembedfs/sub
var files embed.FS

//go:embed goembedfs/one
var one string

var empty embed.FS

func main() {
	err := fs.WalkDir(files, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		fmt.Printf("%-28s dir=%v mode=%v size=%d name=%s\n", p, d.IsDir(), info.Mode(), info.Size(), info.Name())
		return nil
	})
	fmt.Println("walk:", err)

	b, err := files.ReadFile("goembedfs/dir/deep/y.txt")
	fmt.Printf("%q %v\n", b, err)
	_, err = files.ReadFile("goembedfs/dir/.dot")
	fmt.Println(errors.Is(err, fs.ErrNotExist), err)
	_, err = files.ReadFile("goembedfs/dir")
	fmt.Println(err)
	_, err = files.Open("/goembedfs")
	fmt.Println(err)

	ents, err := files.ReadDir("goembedfs")
	for _, e := range ents {
		fmt.Print(e.Name(), " ", e.IsDir(), "; ")
	}
	fmt.Println(err)

	f, err := files.Open("goembedfs/sub/z.txt")
	if err != nil {
		panic(err)
	}
	st, _ := f.Stat()
	data, _ := io.ReadAll(f)
	fmt.Println(st.Name(), st.Size(), st.Mode(), st.ModTime().IsZero(), st.IsDir(), string(data))
	f.Close()

	sub, err := fs.Sub(files, "goembedfs/dir")
	fmt.Println(err)
	m, err := fs.Glob(sub, "*/*")
	fmt.Println(m, err)
	all, _ := fs.Glob(files, "goembedfs/*")
	sort.Strings(all)
	fmt.Println(all)

	fmt.Printf("one: %q\n", one)
	ents, err = empty.ReadDir(".")
	fmt.Println(len(ents), err)
}
