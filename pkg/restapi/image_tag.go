package restapi

import (
	"encoding/json"
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

type ImageTagResponse []string

func (h *imageTagHandler) getImageTags(w http.ResponseWriter, r *http.Request) {
	resp, err := h.tagStorage.GetAll(h.chServerImage)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	_ = json.NewEncoder(w).Encode(resp)
}
