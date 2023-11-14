package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

func (c Client) InitiateRun(cfg InitiateRunConfig) (*url.URL, error) {
	endpoint := "/api/runs"

	if err := cfg.Validate(); err != nil {
		// TODO: Wrap
		return nil, err
	}

	encodedBody, err := json.Marshal(struct{ Run InitiateRunConfig }{cfg})
	if err != nil {
		// TODO: Wrap
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewBuffer(encodedBody))
	if err != nil {
		// TODO: Wrap
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.RoundTrip(req)
	if err != nil {
		// TODO: Wrap
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		msg := extractErrorMessage(resp.Body)
		if msg == "" {
			msg = fmt.Sprintf("Unable to call Mint API - %s", resp.Status)
		}

		return nil, errors.New(msg)
	}

	respBody := struct {
		RunURL string
	}{}

	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		// TODO: Wrap
		return nil, err
	}

	runURL, err := url.Parse(respBody.RunURL)
	if err != nil {
		// TODO: Wrap
		return nil, err
	}

	return runURL, nil
}

func extractErrorMessage(reader io.Reader) string {
	errorStruct := struct {
		Error struct {
			Message string
		}
	}{}

	if err := json.NewDecoder(reader).Decode(&errorStruct); err != nil {
		return ""
	}

	return errorStruct.Error.Message
}
