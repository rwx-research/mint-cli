package cli

import (
	"github.com/rwx-research/mint-cli/internal/api"

	"golang.org/x/crypto/ssh"
)

type APIClient interface {
	GetDebugConnectionInfo(runID string) (api.DebugConnectionInfo, error)
	InitiateRun(api.InitiateRunConfig) (*api.InitiateRunResult, error)
	ObtainAuthCode(api.ObtainAuthCodeConfig) (*api.ObtainAuthCodeResult, error)
	AcquireToken(tokenUrl string) (*api.AcquireTokenResult, error)
}

type SSHClient interface {
	Close() error
	Connect(addr string, cfg ssh.ClientConfig) error
	InteractiveSession() error
}
