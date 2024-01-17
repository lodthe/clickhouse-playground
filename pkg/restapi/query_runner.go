package restapi

import (
	"clickhouse-playground/internal/runsettings"
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

const (
	ClickHouseDatabase = "clickhouse"
)

type queryHandler struct {
	r       QueryRunner
	runRepo queryrun.Repository

	tagStorage TagStorage

	maxQueryLength  uint64
	maxOutputLength uint64
}

func newQueryHandler(r QueryRunner, runRepo queryrun.Repository, storage TagStorage, maxQueryLength, maxOutputLength uint64) *queryHandler {
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
	Query    string      `json:"query"`
	Version  string      `json:"version"`
	Database string      `json:"database"`
	Settings RunSettings `json:"settings,omitempty"`
}

type RunSettings struct {
	ClickHouseSettings *ClickHouseSettings `json:"clickhouse"`
}

type ClickHouseSettings struct {
	OutputFormat string `json:"output_format,omitempty"`
}

type RunQueryOutput struct {
	QueryRunID  string `json:"query_run_id"`
	Output      string `json:"output"`
	TimeElapsed string `json:"time_elapsed"`
}

func convertSettings(req *RunQueryInput) (runsettings.RunSettings, error) {
	var runSettings runsettings.RunSettings

	switch req.Database {
	// TODO: fix after move to OpenAPI
	case ClickHouseDatabase:
		if req.Settings.ClickHouseSettings == nil {
			return nil, nil
		}

		runSettings = &runsettings.ClickHouseSettings{
			OutputFormat: req.Settings.ClickHouseSettings.OutputFormat,
		}

	default:
		return nil, ErrUnknownDatabase
	}

	return runSettings, nil
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

	// Set default database for backward compatibility
	if req.Database == "" {
		req.Database = ClickHouseDatabase
	}

	runSettings, err := convertSettings(&req)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	run := queryrun.New(req.Query, req.Database, req.Version, runSettings)

	startedAt := time.Now()
	output, err := h.r.RunQuery(r.Context(), run)
	if err != nil {
		zlog.Error().Err(err).Interface("request", req).Msg("query run failed")

		switch {
		case errors.Is(err, qrunner.ErrNoAvailableRunners):
			writeError(w, err.Error(), http.StatusTooManyRequests)

		default:
			writeError(w, "internal error", http.StatusInternalServerError)
		}

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
	QueryRunID string                  `json:"query_run_id"`
	Database   string                  `json:"database,omitempty"`
	Version    string                  `json:"version"`
	Settings   runsettings.RunSettings `json:"settings,omitempty"`
	Input      string                  `json:"input"`
	Output     string                  `json:"output"`
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
		Database:   run.Database,
		Version:    run.Version,
		Settings:   run.Settings,
		Input:      run.Input,
		Output:     run.Output,
	})
}
