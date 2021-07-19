package iox

type ReaderFunc func([]byte) (int, error)
type WriterFunc func([]byte) (int, error)

func (f ReaderFunc) Read(b []byte) (int, error)  { return f(b) }
func (f WriterFunc) Write(b []byte) (int, error) { return f(b) }
