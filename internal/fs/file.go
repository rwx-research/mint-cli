package fs

import "io"

type File interface {
	io.ReadWriteCloser
}
