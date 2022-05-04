package dockerhub

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/pkg/errors"
	"go.uber.org/ratelimit"
)

const DockerHubURL = "https://hub.docker.com/v2"
const DefaultMaxRPS = 5

type Client struct {
	apiURL string
	rl     ratelimit.Limiter

	cli *http.Client
}

func NewClient(apiURL string, maxRPS int, httpCli ...*http.Client) *Client {
	c := &Client{
		apiURL: apiURL,
		rl:     ratelimit.New(maxRPS),
		cli:    http.DefaultClient,
	}
	if len(httpCli) == 1 {
		c.cli = httpCli[0]
	}

	return c
}

// GetTags fetches tags of the given image.
func (c *Client) GetTags(image string) ([]ImageTag, error) {
	nextURL := fmt.Sprintf("%s/repositories/%s/tags/", c.apiURL, image)

	var tags []ImageTag
	for {
		resp, err := c.getTags(nextURL)
		if err != nil {
			return nil, err
		}

		tags = append(tags, resp.Results...)
		if resp.Next == nil {
			break
		}

		nextURL = *resp.Next
	}

	return tags, nil
}

func (c *Client) getTags(url string) (*GetImageTagsResponse, error) {
	c.rl.Take()

	resp, err := c.cli.Get(url)
	if err != nil {
		return nil, errors.Wrap(err, "request failed")
	}
	defer resp.Body.Close()

	response := new(GetImageTagsResponse)
	err = json.NewDecoder(resp.Body).Decode(response)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal failed")
	}

	return response, nil
}
