package mocks

import (
	"github.com/rwx-research/mint-cli/internal/fs"

	"github.com/pkg/errors"
)

type FileSystem struct {
	MockCreate   func(name string) (fs.File, error)
	MockOpen     func(name string) (fs.File, error)
	MockReadDir  func(name string) ([]fs.DirEntry, error)
	MockMkdirAll func(path string) error
}

func (f *FileSystem) Create(name string) (fs.File, error) {
	if f.MockCreate != nil {
		return f.MockCreate(name)
	}

	return nil, errors.New("MockCreate was not configured")
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

func (f *FileSystem) MkdirAll(path string) error {
	if f.MockMkdirAll != nil {
		return f.MockMkdirAll(path)
	}

	return errors.New("MockMkdirAll was not configured")
}
