package mocks

import (
	"errors"

	"github.com/rwx-research/mint-cli/internal/client"
)

type Client struct {
	MockInitiateRun func(client.InitiateRunConfig) error
}

func (c *Client) InitiateRun(cfg client.InitiateRunConfig) error {
	if c.MockInitiateRun != nil {
		return c.MockInitiateRun(cfg)
	}

	// TODO: Custom error type?
	return errors.New("MockInitiateRun was not configured")
}
