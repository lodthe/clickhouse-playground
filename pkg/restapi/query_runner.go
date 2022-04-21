package restapi

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"clickhouse-playground/internal/qrunner"
	"clickhouse-playground/internal/queryrun"

	"github.com/go-chi/chi/v5"
)

type queryHandler struct {
	r       qrunner.Runner
	runRepo queryrun.Repository

	tagStorage    TagStorage
	chServerImage string
}

func newQueryHandler(r qrunner.Runner, runRepo queryrun.Repository, storage TagStorage, chServerImage string) *queryHandler {
	return &queryHandler{
		r:             r,
		runRepo:       runRepo,
		tagStorage:    storage,
		chServerImage: chServerImage,
	}
}

func (h *queryHandler) handle(r chi.Router) {
	r.Post("/queries", h.runQuery)
}

type RunQueryRequest struct {
	Query   string `json:"query"`
	Version string `json:"version"`
}

type RunQueryResponse struct {
	Output      string `json:"output"`
	TimeElapsed string `json:"time_elapsed"`
}

func (h *queryHandler) runQuery(w http.ResponseWriter, r *http.Request) {
	var req RunQueryRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	exists, err := h.tagStorage.Exists(h.chServerImage, req.Version)
	if err != nil {
		log.Printf("failed to check tag '%s' existence: %v\n", req.Version, err)
		http.Error(w, "internal error", http.StatusInternalServerError)

		return
	}

	if !exists {
		http.Error(w, "unknown version", http.StatusBadRequest)
		return
	}

	startedAt := time.Now()
	output, err := h.r.RunQuery(r.Context(), req.Query, req.Version)
	if err != nil {
		log.Printf("query run failed: %v\n", err)
		http.Error(w, "internal error", http.StatusInternalServerError)

		return
	}

	run := queryrun.New(req.Query)
	run.Output = output
	err = h.runRepo.Create(run)
	if err != nil {
		log.Printf("failed to save query run: %v\n", err)
		http.Error(w, "internal error", http.StatusInternalServerError)

		return
	}

	resp := RunQueryResponse{
		Output:      output,
		TimeElapsed: time.Since(startedAt).Round(time.Millisecond).String(),
	}

	_ = json.NewEncoder(w).Encode(resp)
}
