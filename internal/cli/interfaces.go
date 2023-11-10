package cli

import (
	"github.com/rwx-research/mint-cli/internal/client"
	"github.com/rwx-research/mint-cli/internal/fs"
)

type FileSystem interface {
	Open(name string) (fs.File, error)
}

type MintClient interface {
	InitiateRun(client.InitiateRunConfig) error
}
