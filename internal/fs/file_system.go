package fs

type FileSystem interface {
	Create(name string) (File, error)
	Open(name string) (File, error)
	ReadDir(name string) ([]DirEntry, error)
	MkdirAll(path string) error
	Getwd() (string, error)
	Exists(name string) (bool, error)
	Stat(name string) (DirEntry, error)
}
