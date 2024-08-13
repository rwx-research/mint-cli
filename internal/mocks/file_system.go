package mocks

import (
	"github.com/rwx-research/mint-cli/internal/errors"
	"github.com/rwx-research/mint-cli/internal/fs"
)

type FileSystem struct {
	MockCreate   func(name string) (fs.File, error)
	MockOpen     func(name string) (fs.File, error)
	MockReadDir  func(name string) ([]fs.DirEntry, error)
	MockMkdirAll func(path string) error
	MockGetwd    func() (string, error)
	MockExists   func(name string) (bool, error)
	MockStat     func(name string) (fs.DirEntry, error)
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

func (f *FileSystem) Getwd() (string, error) {
	if f.MockGetwd != nil {
		return f.MockGetwd()
	}

	return "", errors.New("MockGetwd was not configured")
}

func (f *FileSystem) Exists(name string) (bool, error) {
	if f.MockExists != nil {
		return f.MockExists(name)
	}

	if f.MockStat != nil {
		_, err := f.MockStat(name)
		return err == nil, err
	}

	return false, errors.New("MockExists was not configured")
}

func (f *FileSystem) Stat(name string) (fs.DirEntry, error) {
	if f.MockStat != nil {
		return f.MockStat(name)
	}

	return nil, errors.New("MockStat was not configured")
}
