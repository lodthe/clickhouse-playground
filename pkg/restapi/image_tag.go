package restapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

type imageTagHandler struct {
	tagStorage TagStorage
}

func newImageTagHandler(storage TagStorage) *imageTagHandler {
	return &imageTagHandler{
		tagStorage: storage,
	}
}

func (h *imageTagHandler) handle(r chi.Router) {
	r.Get("/tags", h.getImageTags)
}

type GetImageTagsOutput struct {
	Tags []string `json:"tags"`
}

func (h *imageTagHandler) getImageTags(w http.ResponseWriter, _ *http.Request) {
	tags := h.tagStorage.GetAll()

	names := make([]string, 0, len(tags))
	for _, t := range tags {
		names = append(names, t.Tag)
	}

	writeResult(w, GetImageTagsOutput{Tags: names})
}
