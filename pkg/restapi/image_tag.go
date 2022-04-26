package restapi

import (
	"log"
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
	tags, err := h.tagStorage.GetAll(h.chServerImage)
	if err != nil {
		log.Printf("failed to get image tags: %v\n", err)
		writeError(w, err.Error(), http.StatusInternalServerError)

		return
	}

	writeResult(w, GetImageTagsOutput{Tags: tags})
}
