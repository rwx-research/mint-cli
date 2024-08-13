package memoryfs

import (
	"fmt"
	iofs "io/fs"
	"maps"
	"path"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/rwx-research/mint-cli/internal/fs"
)

const Separator = "/"

var _ fs.FileSystem = (*MemoryFS)(nil)

var (
	ErrExist    = iofs.ErrExist
	ErrNotExist = iofs.ErrNotExist
)

type MemoryFS struct {
	wd      string
	entries map[string]*MemFile
	mu      sync.RWMutex
}

func NewFS() *MemoryFS {
	return &MemoryFS{
		wd: "/",
		entries: map[string]*MemFile{
			"/": {Mode: iofs.ModeDir, ModTime: time.Now()},
		},
	}
}

func (mfs *MemoryFS) Create(name string) (fs.File, error) {
	fullPath := mfs.abs(name)
	mfs.mu.Lock()
	defer mfs.mu.Unlock()

	if _, ok := mfs.entries[fullPath]; ok {
		return nil, ErrExist
	}

	if parent := mfs.lookup(path.Dir(fullPath)); parent == nil {
		return nil, fmt.Errorf("parent directory doesn't exist at %q", path.Dir(name))
	}

	mfs.entries[fullPath] = &MemFile{ModTime: time.Now()}
	return mfs.open(fullPath)
}

func (mfs *MemoryFS) Open(name string) (fs.File, error) {
	mfs.mu.RLock()
	defer mfs.mu.RUnlock()
	return mfs.open(name)
}

func (mfs *MemoryFS) open(name string) (fs.File, error) {
	file := mfs.lookup(name)
	if file == nil {
		return nil, &iofs.PathError{Op: "open", Path: name, Err: ErrNotExist}
	}
	if file.IsDir() {
		return nil, fmt.Errorf("path %q is a directory", name)
	}

	return file.mf.Open()
}

func (mfs *MemoryFS) ReadDir(name string) ([]fs.DirEntry, error) {
	fullPath := mfs.abs(name)
	mfs.mu.RLock()
	defer mfs.mu.RUnlock()

	info := mfs.lookup(fullPath)
	if info == nil {
		return nil, ErrNotExist
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("path %q is not a directory", name)
	}

	prefix := fullPath + "/"
	entries := make([]fs.DirEntry, 0)
	for entryPath, entry := range mfs.entries {
		if strings.HasPrefix(entryPath, prefix) {
			entries = append(entries, &memFileInfo{
				name: entryPath[len(prefix):],
				mf:   entry,
			})
		}
	}

	slices.SortFunc(entries, func(a, b fs.DirEntry) int {
		return strings.Compare(a.Name(), b.Name())
	})

	return entries, nil
}

func (mfs *MemoryFS) MkdirAll(path string) error {
	fullPath := mfs.abs(path)
	mfs.mu.Lock()
	defer mfs.mu.Unlock()

	parts := strings.Split(fullPath, Separator)

	for i := 0; i < len(parts); i++ {
		dir := strings.Join(parts[:i+1], Separator)
		info := mfs.lookup(dir)
		if info != nil {
			if info.IsDir() {
				continue
			}
			return fmt.Errorf("unable to create subdirectory of regular file at %q", dir)
		}

		mfs.entries[dir] = &MemFile{
			Mode:    iofs.ModeDir,
			ModTime: time.Now(),
		}
	}

	return nil
}

func (mfs *MemoryFS) Getwd() (string, error) {
	return mfs.wd, nil
}

func (mfs *MemoryFS) Chdir(cwd string) error {
	mfs.mu.Lock()
	defer mfs.mu.Unlock()

	cwd = path.Clean(cwd)
	if info := mfs.lookup(cwd); info != nil {
		mfs.wd = cwd
		return nil
	}

	return ErrNotExist
}

func (mfs *MemoryFS) Exists(name string) (bool, error) {
	if info := mfs.lookup(name); info != nil {
		return true, nil
	}
	return false, nil
}

func (mfs *MemoryFS) Stat(name string) (fs.DirEntry, error) {
	if info := mfs.lookup(name); info != nil {
		return info, nil
	}

	return nil, ErrNotExist
}

func (mfs *MemoryFS) lookup(name string) *memFileInfo {
	fullPath := mfs.abs(name)

	if file, ok := mfs.entries[fullPath]; ok {
		return &memFileInfo{
			name: path.Base(fullPath),
			mf:   file,
		}
	}

	return nil
}

func (mfs *MemoryFS) abs(name string) string {
	name = strings.TrimSuffix(name, Separator)
	if path.IsAbs(name) {
		return name
	}
	if name == "." {
		return mfs.wd
	}

	return path.Clean(path.Join(mfs.wd, name))
}

// Entries exposes underlying entries for testing.
func (mfs *MemoryFS) Entries() map[string]*MemFile {
	return maps.Clone(mfs.entries)
}

// WriteFiles creates the given files, including any necessary directory structure.
func (mfs *MemoryFS) WriteFiles(files map[string][]byte) error {
	for name, contents := range files {
		name = mfs.abs(name)
		dir := path.Dir(name)
		if err := mfs.MkdirAll(dir); err != nil {
			return err
		}

		file, err := mfs.Create(name)
		if err != nil {
			return err
		}
		if _, err = file.Write(contents); err != nil {
			return err
		}
		if err = file.Close(); err != nil {
			return err
		}
	}
	return nil
}
