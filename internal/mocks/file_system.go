package mocks

import (
	"errors"

	"github.com/rwx-research/mint-cli/internal/fs"
)

type FileSystem struct {
	MockOpen    func(name string) (fs.File, error)
	MockReadDir func(name string) ([]fs.DirEntry, error)
}

func (f *FileSystem) Open(name string) (fs.File, error) {
	if f.MockOpen != nil {
		return f.MockOpen(name)
	}

	// TODO: Custom error type?
	return nil, errors.New("MockOpen was not configured")
}

func (f *FileSystem) ReadDir(name string) ([]fs.DirEntry, error) {
	if f.MockReadDir != nil {
		return f.MockReadDir(name)
	}

	// TODO: Custom error type?
	return nil, errors.New("MockReadDir was not configured")
}
