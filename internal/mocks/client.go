package mocks

import (
	"github.com/rwx-research/mint-cli/internal/client"
	"github.com/rwx-research/mint-cli/internal/errors"
)

type Client struct {
	MockInitiateRun            func(client.InitiateRunConfig) (*client.InitiateRunResult, error)
	MockGetDebugConnectionInfo func(runID string) (client.DebugConnectionInfo, error)
	MockObtainAuthCode         func(client.ObtainAuthCodeConfig) (*client.ObtainAuthCodeResult, error)
	MockAcquireToken           func(tokenUrl string) (*client.AcquireTokenResult, error)
}

func (c *Client) InitiateRun(cfg client.InitiateRunConfig) (*client.InitiateRunResult, error) {
	if c.MockInitiateRun != nil {
		return c.MockInitiateRun(cfg)
	}

	return nil, errors.New("MockInitiateRun was not configured")
}

func (c *Client) GetDebugConnectionInfo(runID string) (client.DebugConnectionInfo, error) {
	if c.MockGetDebugConnectionInfo != nil {
		return c.MockGetDebugConnectionInfo(runID)
	}

	return client.DebugConnectionInfo{}, errors.New("MockGetDebugConnectionInfo was not configured")
}

func (c *Client) ObtainAuthCode(cfg client.ObtainAuthCodeConfig) (*client.ObtainAuthCodeResult, error) {
	if c.MockObtainAuthCode != nil {
		return c.MockObtainAuthCode(cfg)
	}

	return nil, errors.New("MockObtainAuthCode was not configured")
}

func (c *Client) AcquireToken(tokenUrl string) (*client.AcquireTokenResult, error) {
	if c.MockAcquireToken != nil {
		return c.MockAcquireToken(tokenUrl)
	}

	return nil, errors.New("MockAcquireToken was not configured")
}
