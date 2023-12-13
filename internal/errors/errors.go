package errors

import (
	"os"

	"github.com/pkg/errors"
)

var (
	ErrFileNotExists = os.ErrNotExist

	Errorf    = errors.Errorf
	Is        = errors.Is
	New       = errors.New
	WithStack = errors.WithStack
	Wrap      = errors.Wrap
	Wrapf     = errors.Wrapf
)
