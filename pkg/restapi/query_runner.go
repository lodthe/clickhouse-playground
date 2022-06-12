package restapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"clickhouse-playground/internal/qrunner"
	"clickhouse-playground/internal/queryrun"

	"github.com/go-chi/chi/v5"
	"github.com/pkg/errors"
	zlog "github.com/rs/zerolog/log"
)

type queryHandler struct {
	r       qrunner.Runner
	runRepo queryrun.Repository

	tagStorage TagStorage

	maxQueryLength  uint64
	maxOutputLength uint64
}

func newQueryHandler(r qrunner.Runner, runRepo queryrun.Repository, storage TagStorage, maxQueryLength, maxOutputLength uint64) *queryHandler {
	return &queryHandler{
		r:               r,
		runRepo:         runRepo,
		tagStorage:      storage,
		maxQueryLength:  maxQueryLength,
		maxOutputLength: maxOutputLength,
	}
}

func (h *queryHandler) handle(r chi.Router) {
	r.Post("/runs", h.runQuery)
	r.Get("/runs/{id}", h.getQueryRun)
}

type RunQueryInput struct {
	Query   string `json:"query"`
	Version string `json:"version"`
}

type RunQueryOutput struct {
	QueryRunID  string `json:"query_run_id"`
	Output      string `json:"output"`
	TimeElapsed string `json:"time_elapsed"`
}

func (h *queryHandler) runQuery(w http.ResponseWriter, r *http.Request) {
	var req RunQueryInput
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Query == "" {
		writeError(w, "query cannot be empty", http.StatusBadRequest)
		return
	}
	if uint64(len(req.Query)) > h.maxQueryLength {
		msg := fmt.Sprintf("query length (%d) cannot exceed %d", len(req.Query), h.maxQueryLength)
		writeError(w, msg, http.StatusBadRequest)

		return
	}

	if !h.tagStorage.Exists(req.Version) {
		writeError(w, "unknown version", http.StatusBadRequest)
		return
	}

	run := queryrun.New(req.Query, req.Version)

	startedAt := time.Now()
	output, err := h.r.RunQuery(r.Context(), run.ID, req.Query, req.Version)
	if err != nil {
		zlog.Error().Err(err).Interface("request", req).Msg("query run failed")
		writeError(w, "internal error", http.StatusInternalServerError)

		return
	}
	if uint64(len(output)) > h.maxOutputLength {
		msg := fmt.Sprintf("output length (%d) cannot exceed %d", len(output), h.maxOutputLength)
		writeError(w, msg, http.StatusBadRequest)

		return
	}

	timeElapsed := time.Since(startedAt)
	run.Output = output
	run.ExecutionTime = timeElapsed

	err = h.runRepo.Create(run)
	if err != nil {
		zlog.Error().Err(err).Interface("model", run).Msg("a run cannot be saved")
		writeError(w, "internal error", http.StatusInternalServerError)

		return
	}

	zlog.Info().Str("id", run.ID).Dur("elapsed", timeElapsed).Msg("saved a new run")

	writeResult(w, RunQueryOutput{
		QueryRunID:  run.ID,
		Output:      run.Output,
		TimeElapsed: timeElapsed.Round(time.Millisecond).String(),
	})
}

type GetQueryRunInput struct {
	ID string `json:"id"`
}

type GetQueryRunOutput struct {
	QueryRunID string `json:"query_run_id"`
	Version    string `json:"version"`
	Input      string `json:"input"`
	Output     string `json:"output"`
}

func (h *queryHandler) getQueryRun(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, "missed id", http.StatusBadRequest)
		return
	}

	run, err := h.runRepo.Get(id)
	if errors.Is(err, queryrun.ErrNotFound) {
		writeError(w, "run not found", http.StatusNotFound)
		return
	}
	if err != nil {
		zlog.Error().Err(err).Str("id", id).Msg("failed to find a run")
		writeError(w, "internal error", http.StatusInternalServerError)

		return
	}

	writeResult(w, GetQueryRunOutput{
		QueryRunID: run.ID,
		Version:    run.Version,
		Input:      run.Input,
		Output:     run.Output,
	})
}
