package dockerhub

import "time"

type GetImageTagsResponse struct {
	Count    int        `json:"count"`
	Next     *string    `json:"next"`
	Previous *string    `json:"previous"`
	Results  []ImageTag `json:"results"`
}

type ImageTag struct {
	Creator             int          `json:"creator"`
	ID                  int          `json:"id"`
	ImageID             *interface{} `json:"image_id"`
	Images              []Image      `json:"images"`
	LastUpdated         time.Time    `json:"last_updated"`
	LastUpdater         int          `json:"last_updater"`
	LastUpdaterUsername string       `json:"last_updater_username"`
	Name                string       `json:"name"`
	Repository          int          `json:"repository"`
	FullSize            int          `json:"full_size"`
	V2                  bool         `json:"v2"`
	TagStatus           string       `json:"tag_status"`
	TagLastPulled       *time.Time   `json:"tag_last_pulled"`
	TagLastPushed       time.Time    `json:"tag_last_pushed"`
}

type Image struct {
	Architecture string     `json:"architecture"`
	Features     *string    `json:"features"`
	Variant      *string    `json:"variant"`
	Digest       string     `json:"digest"`
	OS           string     `json:"os"`
	OSFeatures   *string    `json:"os_features"`
	OSVersion    *string    `json:"os_version"`
	Size         int        `json:"size"`
	Status       string     `json:"status"`
	LastPulled   *time.Time `json:"last_pulled"`
	LastPushed   time.Time  `json:"last_pushed"`
}
