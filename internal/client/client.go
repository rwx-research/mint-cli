package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/pkg/errors"
)

// Client is an API Client for Mint
type Client struct {
	RoundTrip func(*http.Request) (*http.Response, error)
}

func New(cfg Config) (Client, error) {
	if err := cfg.Validate(); err != nil {
		return Client{}, errors.Wrap(err, "validation failed")
	}

	roundTrip := func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = "https"
		req.URL.Host = cfg.Host
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", cfg.AccessToken))

		return http.DefaultClient.Do(req)
	}

	return Client{roundTrip}, nil
}

// InitiateRun sends a request to Mint for starting a new runn
func (c Client) InitiateRun(cfg InitiateRunConfig) (*InitiateRunResult, error) {
	endpoint := "/api/runs"

	if err := cfg.Validate(); err != nil {
		return nil, errors.Wrap(err, "validation failed")
	}

	encodedBody, err := json.Marshal(struct{ Run InitiateRunConfig }{cfg})
	if err != nil {
		return nil, errors.Wrap(err, "unable to encode as JSON")
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewBuffer(encodedBody))
	if err != nil {
		return nil, errors.Wrap(err, "unable to create new HTTP request")
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.RoundTrip(req)
	if err != nil {
		return nil, errors.Wrap(err, "HTTP request failed")
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
		RunId            string
		RunURL           string
		TargetedTaskKeys []string
		DefinitionPath   string
	}{}

	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		return nil, errors.Wrap(err, "unable to parse API response")
	}

	return &InitiateRunResult{
		RunId: respBody.RunId,
		RunURL: respBody.RunURL,
		TargetedTaskKeys: respBody.TargetedTaskKeys,
		DefinitionPath: respBody.DefinitionPath,
	}, nil
}

// extractErrorMessage is a small helper function for parsing an API error message
func extractErrorMessage(reader io.Reader) string {
	errorStruct := struct {
		Result struct {
			Data struct {
				Error string
			}
		}
	}{}

	if err := json.NewDecoder(reader).Decode(&errorStruct); err != nil {
		return ""
	}

	return errorStruct.Result.Data.Error
}
