package mocks

import (
	"errors"

	"github.com/rwx-research/mint-cli/internal/fs"
)

type FileSystem struct {
	MockOpen func(name string) (fs.File, error)
}

func (f *FileSystem) Open(name string) (fs.File, error) {
	if f.MockOpen != nil {
		return f.MockOpen(name)
	}

	// TODO: Custom error type?
	return nil, errors.New("MockInitiateRun was not configured")
}
