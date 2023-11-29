package mocks

import (
	"github.com/rwx-research/mint-cli/internal/client"

	"github.com/pkg/errors"
)

type Client struct {
	MockInitiateRun func(client.InitiateRunConfig) (*client.InitiateRunResult, error)
}

func (c *Client) InitiateRun(cfg client.InitiateRunConfig) (*client.InitiateRunResult, error) {
	if c.MockInitiateRun != nil {
		return c.MockInitiateRun(cfg)
	}

	return nil, errors.New("MockInitiateRun was not configured")
}
