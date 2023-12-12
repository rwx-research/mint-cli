package mocks

import (
	"bytes"
)

type File struct {
	*bytes.Buffer
}

func NewFile(content string) *File {
	file := new(File)
	file.Buffer = bytes.NewBufferString(content)
	return file
}

func (f *File) Close() error {
	return nil
}
