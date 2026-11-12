package vm

// searchFlag is O_SEARCH of darwin (O_EXEC|O_DIRECTORY, macOS 13): it opens
// a directory for openat without the right to read it. Older systems fall
// back to O_RDONLY (openDir). createKeepsSticky is false: open drops the
// sticky bit of a new file, as os/sticky_bsd.go says. maxLinks is the
// number of links that openNoAlias follows in the last element: the
// kernel (MAXSYMLINKS 32 in sys/param.h) refuses the 32nd.
const (
	searchFlag        = 0x40100000
	openExtra         = 0
	createKeepsSticky = false
	maxLinks          = 31
)

func isProcFS(int) bool { return false }
