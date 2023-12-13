package accesstoken

import (
	gofs "io/fs"

	"fmt"
	"io"
	"os"
	"os/user"
	"path"
	"strings"

	"github.com/rwx-research/mint-cli/internal/errors"
	"github.com/rwx-research/mint-cli/internal/fs"
)

type FileBackend struct {
	Dir        string
	FileSystem fs.FileSystem
}

func NewFileBackend(dir string, filesystem fs.FileSystem) (*FileBackend, error) {
	dir, err := expandTilde(dir)
	if err != nil {
		return nil, err
	}

	return &FileBackend{Dir: dir, FileSystem: filesystem}, nil
}

func (f FileBackend) Get() (string, error) {
	filepath := path.Join(f.Dir, "accesstoken")
	fd, err := f.FileSystem.Open(path.Join(f.Dir, "accesstoken"))
	if err != nil {
		if errors.Is(err, gofs.ErrNotExist) {
			return "", nil
		}

		return "", errors.Wrapf(err, "unable to open %q", filepath)
	}
	defer fd.Close()

	contents, err := io.ReadAll(fd)
	if err != nil {
		return "", errors.Wrapf(err, "error reading %q", filepath)
	}

	return string(contents), nil
}

func (f FileBackend) Set(value string) error {
	err := f.FileSystem.MkdirAll(f.Dir)
	if err != nil {
		return errors.Wrapf(err, "unable to create %q", f.Dir)
	}

	filepath := path.Join(f.Dir, "accesstoken")
	fd, err := f.FileSystem.Create(path.Join(f.Dir, "accesstoken"))
	if err != nil {
		return errors.Wrapf(err, "unable to create %q", filepath)
	}
	defer fd.Close()

	_, err = io.WriteString(fd, value)
	if err != nil {
		return errors.Wrapf(err, "unable to write token to %q", filepath)
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
		return path.Join(user.HomeDir, strings.TrimPrefix(dir, tildeSlash)), nil
	} else if dir == "~" {
		return user.HomeDir, nil
	} else {
		return dir, nil
	}
}
