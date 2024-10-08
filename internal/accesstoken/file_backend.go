package accesstoken

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/rwx-research/mint-cli/internal/errors"
)

type FileBackend struct {
	Dir string
}

func NewFileBackend(dir string) (*FileBackend, error) {
	dir, err := expandTilde(dir)
	if err != nil {
		return nil, err
	}

	return &FileBackend{Dir: dir}, nil
}

func (f FileBackend) Get() (string, error) {
	path := filepath.Join(f.Dir, "accesstoken")
	fd, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}

		return "", errors.Wrapf(err, "unable to open %q", path)
	}
	defer fd.Close()

	contents, err := io.ReadAll(fd)
	if err != nil {
		return "", errors.Wrapf(err, "error reading %q", path)
	}

	return strings.TrimSpace(string(contents)), nil
}

func (f FileBackend) Set(value string) error {
	err := os.MkdirAll(f.Dir, os.ModePerm)
	if err != nil {
		return errors.Wrapf(err, "unable to create %q", f.Dir)
	}

	path := filepath.Join(f.Dir, "accesstoken")
	fd, err := os.Create(path)
	if err != nil {
		return errors.Wrapf(err, "unable to create %q", path)
	}
	defer fd.Close()

	_, err = io.WriteString(fd, value)
	if err != nil {
		return errors.Wrapf(err, "unable to write token to %q", path)
	}

	return nil
}

var tildeSlash = fmt.Sprintf("~%v", string(os.PathSeparator))

func expandTilde(dir string) (string, error) {
	user, err := user.Current()
	if err != nil {
		return "", err
	}

	if strings.HasPrefix(dir, tildeSlash) {
		return filepath.Join(user.HomeDir, strings.TrimPrefix(dir, tildeSlash)), nil
	} else if dir == "~" {
		return user.HomeDir, nil
	} else {
		return dir, nil
	}
}
