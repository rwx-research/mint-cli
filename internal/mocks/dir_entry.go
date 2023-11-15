package mocks

type DirEntry struct {
	FileName    string
	IsDirectory bool
}

func (d DirEntry) Name() string {
	return d.FileName
}

func (d DirEntry) IsDir() bool {
	return d.IsDirectory
}
