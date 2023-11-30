package mocks

import (
	"net/url"

	"github.com/rwx-research/mint-cli/internal/client"

	"github.com/pkg/errors"
)

type Client struct {
	MockInitiateRun            func(client.InitiateRunConfig) (*url.URL, error)
	MockGetDebugConnectionInfo func(runID string) (client.DebugConnectionInfo, error)
}

func (c *Client) InitiateRun(cfg client.InitiateRunConfig) (*url.URL, error) {
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
