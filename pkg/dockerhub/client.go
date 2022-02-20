package dockerhub

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/pkg/errors"
)

const URL = "https://registry.hub.docker.com/v1"

type Client struct {
	cli *http.Client
}

func NewClient(httpCli ...*http.Client) *Client {
	c := &Client{
		cli: http.DefaultClient,
	}
	if len(httpCli) == 1 {
		c.cli = httpCli[0]
	}

	return c
}

// GetTags fetches tags of the given image.
// The returned list of tags is a reversed version of the response, the "latest" tag has the first place.
func (c *Client) GetTags(image string) ([]string, error) {
	resp, err := c.cli.Get(fmt.Sprintf("%s/repositories/%s/tags", URL, image))
	if err != nil {
		return nil, errors.Wrap(err, "request failed")
	}
	defer resp.Body.Close()

	var list []TagList
	err = json.NewDecoder(resp.Body).Decode(&list)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal failed")
	}

	// Reverse the list of tags and put the "latest" in the first place.
	var hasLatest bool
	tags := make([]string, 0, len(list))
	for _, i := range list {
		if i.Name == "latest" {
			hasLatest = true
			continue
		}

		tags = append(tags, i.Name)
	}

	for i, j := 0, len(tags)-1; i < j; i, j = i+1, j-1 {
		tags[i], tags[j] = tags[j], tags[i]
	}

	if hasLatest {
		tags = append(tags, "latest")
	}

	return tags, nil
}
