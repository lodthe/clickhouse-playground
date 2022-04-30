package restapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	zlog "github.com/rs/zerolog/log"
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
		zlog.Error().Err(err).Str("image_tag", h.chServerImage).Msg("failed to get image tags")
		writeError(w, err.Error(), http.StatusInternalServerError)

		return
	}

	writeResult(w, GetImageTagsOutput{Tags: tags})
}
