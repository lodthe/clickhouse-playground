package playgroundclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"clickhouse-playground/pkg/restapi"
)

type Config struct {
	BaseURL string
}

type PlaygroundClient struct {
	baseURL string
	client  *http.Client
}

type PostRunsResponse struct {
	Result restapi.RunQueryOutput `json:"result"`
}

func New(c *Config) *PlaygroundClient {
	return &PlaygroundClient{
		baseURL: c.BaseURL,
		client:  &http.Client{Timeout: 0},
	}
}

func (c *PlaygroundClient) PostRuns(database string, version string, query string) (time.Duration, error) {
	url := c.baseURL + "/api/runs"

	requestBody := restapi.RunQueryInput{
		Database:    database,
		Version:     version,
		Query:       query,
		SaveRunInfo: false,
	}
	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal request body")
	}

	inp, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return 0, fmt.Errorf("invalid request: %w", err)
	}

	response, err := c.client.Do(inp)
	if err != nil {
		return 0, fmt.Errorf("request failed: %w", err)
	}

	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return 0, fmt.Errorf("can't parse response body: %w", err)
	}

	if response.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("received unsuccessful status from playground client: %s", body)
	}

	var result PostRunsResponse
	err = json.Unmarshal(body, &result)
	if err != nil {
		return 0, fmt.Errorf("invalid body response: %w", err)
	}

	duration, err := time.ParseDuration(result.Result.TimeElapsed)
	if err != nil {
		return 0, fmt.Errorf("can't parse elapsed time: %w", err)
	}

	return duration, nil
}
