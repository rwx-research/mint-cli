package fs

import (
	"os"

	"github.com/pkg/errors"
)

type Local struct{}

func (l Local) Create(name string) (File, error) {
	fd, err := os.Create(name)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to create %q", name)
	}

	return fd, nil
}

func (l Local) Open(name string) (File, error) {
	fd, err := os.Open(name)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to open %q", name)
	}

	return fd, nil
}

func (l Local) ReadDir(name string) ([]DirEntry, error) {
	files, err := os.ReadDir(name)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to read %q", name)
	}

	entries := make([]DirEntry, len(files))
	for i, file := range files {
		entries[i] = file
	}

	return entries, nil
}

func (l Local) MkdirAll(path string) error {
	return os.MkdirAll(path, os.ModePerm)
}
