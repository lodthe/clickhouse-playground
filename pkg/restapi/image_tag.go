package restapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

type imageTagHandler struct {
	tagStorage    TagStorage
	chServerImage string
}

func newImageTagHandler(storage TagStorage, chServerImage string) *imageTagHandler {
	return &imageTagHandler{
		tagStorage:    storage,
		chServerImage: chServerImage,
	}
}

func (h *imageTagHandler) handle(r chi.Router) {
	r.Get("/tags", h.getImageTags)
}

type GetImageTagsOutput struct {
	Tags []string `json:"tags"`
}

func (h *imageTagHandler) getImageTags(w http.ResponseWriter, r *http.Request) {
	tags := h.tagStorage.GetAll()

	names := make([]string, 0, len(tags))
	for _, t := range tags {
		names = append(names, t.TagName)
	}

	writeResult(w, GetImageTagsOutput{Tags: names})
}
