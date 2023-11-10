package fs

import "os"

type Local struct{}

func (l Local) Open(name string) (File, error) {
	// TODO: Wrap?
	return os.Open(name)
}
