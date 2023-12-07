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
		It("prefixes the endpoint with the base path", func() {
			body := struct {
				RunID            string   `json:"runId"`
				RunURL           string   `json:"runUrl"`
				TargetedTaskKeys []string `json:"targetedTaskKeys"`
				DefinitionPath   string   `json:"definitionPath"`
			}{
				RunID:            "123",
				RunURL:           "https://cloud.rwx.com/mint/org/runs/123",
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

			c := client.Client{roundTrip}

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
