package fs

type FileSystem interface {
	Create(name string) (File, error)
	Open(name string) (File, error)
	ReadDir(name string) ([]DirEntry, error)
	MkdirAll(path string) error
}
