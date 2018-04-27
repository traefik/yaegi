package export

// Provide access to go standard library

//go:generate go run gen.go archive/tar archive/zip
//go:generate go run gen.go bufio bytes
//go:generate go run gen.go fmt log math os os/exec
//go:generate go run gen.go strings sync time

var Pkg = &map[string]*map[string]interface{}{
	"archive/tar": sym_tar,
	"archive/zip": sym_zip,
	"bufio":       sym_bufio,
	"bytes":       sym_bytes,
	"fmt":         sym_fmt,
	"log":         sym_log,
	"math":        sym_math,
	"os":          sym_os,
	"os/exec":     sym_exec,
	"strings":     sym_strings,
	"sync":        sym_sync,
	"time":        sym_time,
}
