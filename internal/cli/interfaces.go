package cli

import (
	"github.com/rwx-research/mint-cli/internal/client"
	"github.com/rwx-research/mint-cli/internal/fs"
)

type FileSystem interface {
	Open(name string) (fs.File, error)
	ReadDir(name string) ([]fs.DirEntry, error)
}

type MintClient interface {
	InitiateRun(client.InitiateRunConfig) (*client.InitiateRunResult, error)
}
