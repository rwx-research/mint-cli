package fs

type DirEntry interface {
	IsDir() bool
	Name() string
}
