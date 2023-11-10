package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

type Client struct {
	RoundTrip func(*http.Request) (*http.Response, error)
}

func New(cfg Config) (Client, error) {
	if err := cfg.Validate(); err != nil {
		// TODO: Wrap
		return Client{}, err
	}

	roundTrip := func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = "https"
		req.URL.Host = cfg.Host
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", cfg.AccessToken))

		return http.DefaultClient.Do(req)
	}

	return Client{roundTrip}, nil
}

func (c Client) InitiateRun(cfg InitiateRunConfig) error {
	endpoint := "/api/runs"

	if err := cfg.Validate(); err != nil {
		// TODO: Wrap
		return err
	}

	encodedBody, err := json.Marshal(cfg)
	if err != nil {
		// TODO: Wrap
		return err
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewBuffer(encodedBody))
	if err != nil {
		// TODO: Wrap
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.RoundTrip(req)
	if err != nil {
		// TODO: Wrap
		return err
	}

	if resp.StatusCode != 200 {
		// TODO: Custom error?
		body, _ := io.ReadAll(resp.Body)
		return errors.New(fmt.Sprintf("HTTP call unsuccessful (%s): %s", resp.Status, body))
	}

	return nil
}
