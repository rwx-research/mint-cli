package memoryfs

import (
	iofs "io/fs"
	"time"

	"github.com/rwx-research/mint-cli/internal/fs"
)

var _ iofs.FileInfo = (*memFileInfo)(nil)
var _ fs.DirEntry = (*memFileInfo)(nil)

type memFileInfo struct {
	name string
	mf   *MemFile
}

func (fi *memFileInfo) Name() string {
	return fi.name
}

func (fi *memFileInfo) Size() int64 {
	return int64(len(fi.mf.data))
}

func (fi *memFileInfo) Mode() iofs.FileMode {
	return fi.mf.Mode
}

func (fi *memFileInfo) ModTime() time.Time {
	return fi.mf.ModTime
}

func (fi *memFileInfo) IsDir() bool {
	return fi.mf.Mode.IsDir()
}

func (fi *memFileInfo) Sys() any {
	return fi.mf.Sys
}
