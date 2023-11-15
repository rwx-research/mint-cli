package mocks

import (
	"github.com/rwx-research/mint-cli/internal/fs"

	"github.com/pkg/errors"
)

type FileSystem struct {
	MockOpen    func(name string) (fs.File, error)
	MockReadDir func(name string) ([]fs.DirEntry, error)
}

func (f *FileSystem) Open(name string) (fs.File, error) {
	if f.MockOpen != nil {
		return f.MockOpen(name)
	}

	return nil, errors.New("MockOpen was not configured")
}

func (f *FileSystem) ReadDir(name string) ([]fs.DirEntry, error) {
	if f.MockReadDir != nil {
		return f.MockReadDir(name)
	}

	return nil, errors.New("MockReadDir was not configured")
}
