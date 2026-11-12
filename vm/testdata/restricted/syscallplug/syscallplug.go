// Package syscallplug uses the read-only syscall subset of a restricted
// plugin with useUnsafe.
package syscallplug

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"syscall"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	getpid := syscall.Getpid // as a function value too
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var tv syscall.Timeval
		tvErr := syscall.Gettimeofday(&tv)
		v, found := syscall.Getenv("YAEGI_TEST_VAR")
		_, hasPath := syscall.Getenv("PATH")
		n := 0
		for _, kv := range syscall.Environ() {
			if strings.HasPrefix(kv, "YAEGI_TEST_") {
				n++
			}
		}
		var err error = syscall.ENOENT
		var errno syscall.Errno
		ids := syscall.Getuid() == os.Getuid() && syscall.Geteuid() == os.Geteuid() &&
			syscall.Getgid() == os.Getgid() && syscall.Getegid() == os.Getegid()
		// Timespec and the conversions (wazero's sys/stat_bsd.go)
		ts := syscall.NsecToTimespec(1_500_000_001)
		tsOK := ts.Sec == 1 && ts.Nsec == 500_000_001 && syscall.TimespecToNsec(ts) == 1_500_000_001 &&
			ts.Nano() == 1_500_000_001 && timevalOK()
		fmt.Fprintf(w, "pid=%v ppid=%v ids=%v page=%v time=%v env=%s,%v,%v,%d errno=%v timespec=%v",
			syscall.Getpid() == os.Getpid() && getpid() == os.Getpid(),
			syscall.Getppid() == os.Getppid(),
			ids,
			syscall.Getpagesize() == os.Getpagesize(),
			tvErr == nil && tv.Sec > 0,
			v, found, hasPath, n,
			errors.As(err, &errno) && errno == syscall.ENOENT && errors.Is(err, syscall.ENOENT),
			tsOK)
	}), nil
}
