//go:build !unix

package vm

// openNoBlock is the open flag that makes the open of a FIFO return without
// a writer on Unix (see openRegular). The other systems have no FIFO in a
// directory, or no such flag.
const openNoBlock = 0
