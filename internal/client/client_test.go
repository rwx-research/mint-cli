package client_test

import (
	"bytes"
	"encoding/json"
	"io"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"net/http"

	"github.com/rwx-research/mint-cli/internal/client"
)

var _ = Describe("Client", func() {
	Describe("InitiateRun", func() {
		Context("without a mint base path", func() {
			It("uses an mint.rwx.com style endpoint", func() {
				body := struct {
					RunID            string   `json:"runId"`
					RunURL           string   `json:"runUrl"`
					TargetedTaskKeys []string `json:"targetedTaskKeys"`
					DefinitionPath   string   `json:"definitionPath"`
				}{
					RunID:            "123",
					RunURL:           "https://mint.rwx.com/runs/123",
					TargetedTaskKeys: []string{},
					DefinitionPath:   "foo",
				}
				bodyBytes, _ := json.Marshal(body)

				roundTrip := func(req *http.Request) (*http.Response, error) {
					Expect(req.URL.Path).To(Equal("/api/runs"))
					return &http.Response{
						Status:     "201 Created",
						StatusCode: 201,
						Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
					}, nil
				}

				c := client.Client{roundTrip, "mint.rwx.com"}

				initRunConfig := client.InitiateRunConfig{
					InitializationParameters: map[string]string{},
					TaskDefinitions: []client.TaskDefinition{
						{Path: "foo", FileContents: "echo 'bar'"},
					},
					TargetedTaskKeys: []string{},
					UseCache:         false,
				}

				_, err := c.InitiateRun(initRunConfig)
				Expect(err).To(BeNil())
			})
		})

		Context("with a mint base path", func() {
			It("prefixes the endpoint with the base path", func() {
				body := struct {
					RunID            string   `json:"runId"`
					RunURL           string   `json:"runUrl"`
					TargetedTaskKeys []string `json:"targetedTaskKeys"`
					DefinitionPath   string   `json:"definitionPath"`
				}{
					RunID:            "123",
					RunURL:           "https://mint.rwx.com/runs/123",
					TargetedTaskKeys: []string{},
					DefinitionPath:   "foo",
				}
				bodyBytes, _ := json.Marshal(body)

				roundTrip := func(req *http.Request) (*http.Response, error) {
					Expect(req.URL.Path).To(Equal("/mint/api/runs"))
					return &http.Response{
						Status:     "201 Created",
						StatusCode: 201,
						Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
					}, nil
				}

				c := client.Client{roundTrip, "cloud.rwx.com"}

				initRunConfig := client.InitiateRunConfig{
					InitializationParameters: map[string]string{},
					TaskDefinitions: []client.TaskDefinition{
						{Path: "foo", FileContents: "echo 'bar'"},
					},
					TargetedTaskKeys: []string{},
					UseCache:         false,
				}

				_, err := c.InitiateRun(initRunConfig)
				Expect(err).To(BeNil())
			})
		})
	})
})
