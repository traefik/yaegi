package vm

import "testing"

// TestLoggerField runs testdata/loggerfield with the standard library of
// plugins (RestrictedStd, also in AOT mode) and with the restrictions of
// plugins: a logger made by log.New or log.Default can be stored in a native
// field of type *log.Logger and read back, although log.Logger is a wrapper
// type there.
func TestLoggerField(t *testing.T) {
	for _, opts := range []RunOptions{
		{Dir: "..", RestrictedStd: true},
		{Dir: "..", RestrictedStd: true, AOT: true},
		{Dir: "..", Restricted: true},
	} {
		if _, err := RunMain(opts, "./vm/testdata/loggerfield"); err != nil {
			t.Errorf("%+v: %v", opts, err)
		}
	}
}
