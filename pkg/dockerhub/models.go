package dockerhub

type TagList struct {
	Layer string `json:"layer"`
	Name  string `json:"name"`
}
