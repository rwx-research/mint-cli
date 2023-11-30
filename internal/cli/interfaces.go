package cli

import (
	"net/url"

	"github.com/rwx-research/mint-cli/internal/client"
	"github.com/rwx-research/mint-cli/internal/fs"

	"golang.org/x/crypto/ssh"
)

type APIClient interface {
	GetDebugConnectionInfo(runID string) (client.DebugConnectionInfo, error)
	InitiateRun(client.InitiateRunConfig) (*url.URL, error)
}

type FileSystem interface {
	Open(name string) (fs.File, error)
	ReadDir(name string) ([]fs.DirEntry, error)
}

type SSHClient interface {
	Close() error
	Connect(addr string, cfg ssh.ClientConfig) error
	InteractiveSession() error
}
