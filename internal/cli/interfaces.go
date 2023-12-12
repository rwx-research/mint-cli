package cli

import (
	"github.com/rwx-research/mint-cli/internal/client"

	"golang.org/x/crypto/ssh"
)

type APIClient interface {
	GetDebugConnectionInfo(runID string) (client.DebugConnectionInfo, error)
	InitiateRun(client.InitiateRunConfig) (*client.InitiateRunResult, error)
	ObtainAuthCode(client.ObtainAuthCodeConfig) (*client.ObtainAuthCodeResult, error)
	AcquireToken(tokenUrl string) (*client.AcquireTokenResult, error)
}

type SSHClient interface {
	Close() error
	Connect(addr string, cfg ssh.ClientConfig) error
	InteractiveSession() error
}
