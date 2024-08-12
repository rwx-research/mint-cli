package memoryfs

import (
	"bytes"
	"io"
	iofs "io/fs"
	"slices"
	"sync"
	"time"

	"github.com/rwx-research/mint-cli/internal/fs"
)

var _ fs.File = (*openedMemFile)(nil)
var _ iofs.FileInfo = (*memFileInfo)(nil)

var ErrClosed = iofs.ErrClosed

type MemFile struct {
	Mode    iofs.FileMode
	ModTime time.Time
	data    []byte
	Sys     any
}

func (mf *MemFile) Bytes() []byte {
	return bytes.Clone(mf.data)
}

func (mf *MemFile) Open() (*openedMemFile, error) {
	return &openedMemFile{
		mf:  mf,
		buf: mf.Bytes(),
	}, nil
}

func (mf *MemFile) replaceData(data []byte) {
	mf.data = data
	mf.ModTime = time.Now()
}

type openedMemFile struct {
	mf      *MemFile
	buf     []byte
	offset  int
	closed  bool
	changes bool
	mu      sync.Mutex
}

func (fd *openedMemFile) Read(p []byte) (n int, err error) {
	fd.mu.Lock()
	defer fd.mu.Unlock()

	if fd.closed {
		return 0, ErrClosed
	}
	if fd.empty() {
		return 0, io.EOF
	}

	n = copy(p, fd.buf[fd.offset:])
	fd.offset += n

	return n, nil
}

func (fd *openedMemFile) empty() bool {
	return len(fd.buf) <= fd.offset
}

func (fd *openedMemFile) Write(p []byte) (n int, err error) {
	fd.mu.Lock()
	defer fd.mu.Unlock()

	if fd.closed {
		return 0, ErrClosed
	}

	// Grow and reslice
	fd.buf = slices.Grow(fd.buf[:fd.offset], len(p))[:fd.offset+len(p)]

	n = copy(fd.buf[fd.offset:], p)
	fd.offset += n
	fd.changes = true

	return
}

func (fd *openedMemFile) Close() error {
	fd.mu.Lock()
	defer fd.mu.Unlock()

	if fd.closed {
		return ErrClosed
	}

	if fd.changes {
		fd.mf.replaceData(fd.buf)
	}

	fd.closed = true
	return nil
}
