package dockerhub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"go.uber.org/ratelimit"
)

const (
	DockerHubURL    = "https://hub.docker.com/v2"
	DefaultMaxRPS   = 5
	DefaultPageSize = 100
)

// Auth holds information required to obtain an access token:
// https://docs.docker.com/reference/api/hub/latest/#tag/authentication-api/operation/AuthCreateAccessToken
type Auth struct {
	Identifier string `json:"identifier"`
	Secret     string `json:"secret"`
}

type Client struct {
	apiURL string
	auth   Auth

	rl  ratelimit.Limiter
	log zerolog.Logger

	cli *http.Client
}

func NewClient(log zerolog.Logger, apiURL string, maxRPS int, auth Auth, httpCli ...*http.Client) *Client {
	c := &Client{
		apiURL: apiURL,
		auth:   auth,
		rl:     ratelimit.New(maxRPS),
		log:    log,
		cli:    http.DefaultClient,
	}
	if len(httpCli) == 1 {
		c.cli = httpCli[0]
	}

	return c
}

func (c *Client) getAccessToken() (string, error) {
	url := fmt.Sprintf("%s/auth/token", c.apiURL)

	request, err := json.Marshal(c.auth)
	if err != nil {
		return "", fmt.Errorf("json marshal failed: %w", err)
	}

	resp, err := c.cli.Post(url, "application/json", bytes.NewReader(request))
	if err != nil {
		return "", fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	response := struct {
		AccessToken string `json:"access_token"`
	}{}
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		return "", fmt.Errorf("json decode access token response: %w", err)
	}

	return response.AccessToken, nil
}

// GetTags fetches tags of the given image.
func (c *Client) GetTags(repository string) ([]ImageTag, error) {
	startedAt := time.Now()

	token, err := c.getAccessToken()
	if err != nil {
		return nil, fmt.Errorf("failed to acquire an access token: %w", err)
	}

	nextURL := fmt.Sprintf("%s/repositories/%s/tags?page_size=%d", c.apiURL, repository, DefaultPageSize)

	var iterations int
	var tags []ImageTag
	for {
		iterations++
		resp, err := c.getTags(nextURL, token)
		if err != nil {
			return nil, err
		}

		tags = append(tags, resp.Results...)
		if resp.Next == nil {
			break
		}

		nextURL = *resp.Next
	}

	c.log.Info().
		Dur("time_elapsed_ms", time.Since(startedAt)).
		Int("count_api_calls", iterations).
		Str("repository", repository).
		Int("count_image_tags", len(tags)).
		Msg("successfully fetched docker hub tags")

	return tags, nil
}

func (c *Client) getTags(url string, token string) (*GetImageTagsResponse, error) { // nolint
	c.rl.Take()

	req, err := http.NewRequest("GET", url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.cli.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "request failed")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "body read failed")
	}

	response := new(GetImageTagsResponse)

	err = json.Unmarshal(body, response)
	if err != nil {
		c.log.Error().Err(err).Str("url", url).Str("body", string(body)).Msg("failed to fetch image tags")

		return nil, errors.Wrap(err, "unmarshal failed")
	}

	for _, tag := range response.Results {
		if len(tag.Images) == 0 {
			c.log.Warn().Str("api_url", url).Interface("image_tag", tag).
				Msg("got image tag with empty list of images from Docker Hub API; probably there are problems with API calls")
		}
	}

	return response, nil
}
