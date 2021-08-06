package iox

type (
	// ReaderFunc is an io.Reader function.
	ReaderFunc func([]byte) (int, error)

	// WriterFunc is an io.Writer function.
	WriterFunc func([]byte) (int, error)
)

// Read implements io.Reader.
func (f ReaderFunc) Read(b []byte) (int, error) { return f(b) }

// Write implements io.Writer.
func (f WriterFunc) Write(b []byte) (int, error) { return f(b) }
