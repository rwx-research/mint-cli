package fs

import "os"

type Local struct{}

func (l Local) Open(name string) (File, error) {
	// TODO: Wrap
	return os.Open(name)
}

func (l Local) ReadDir(name string) ([]DirEntry, error) {
	files, err := os.ReadDir(name)
	if err != nil {
		// TODO: Wrap
		return nil, err
	}

	entries := make([]DirEntry, len(files))
	for i, file := range files {
		entries[i] = file
	}

	return entries, nil
}
