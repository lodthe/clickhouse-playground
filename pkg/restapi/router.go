package restapi

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"clickhouse-playground/internal/metrics"
	"clickhouse-playground/internal/qrunner"
	"clickhouse-playground/internal/queryrun"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

type RouterOpts struct {
	DockerRepository string
}

func NewRouter(timeout time.Duration, runner qrunner.Runner, tagStorage TagStorage, runRepo queryrun.Repository, maxQueryLength, maxOutputLength uint64) http.Handler {
	r := chi.NewRouter()

	r.Use(metricsMiddleware)

	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Use(middleware.Timeout(timeout))

	r.Use(cors.Handler(cors.Options{
		// AllowedOrigins:   []string{"https://foo.com"}, // Use this to allow specific origin hosts
		AllowedOrigins:   []string{"https://*", "http://*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	r.Route("/api", func(r chi.Router) {
		newQueryHandler(runner, runRepo, tagStorage, maxQueryLength, maxOutputLength).handle(r)
		newImageTagHandler(tagStorage).handle(r)
	})

	return r
}

func metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)

		rctx := chi.RouteContext(r.Context())
		routePattern := strings.Join(rctx.RoutePatterns, "")

		status := fmt.Sprintf("%d %s", ww.Status(), http.StatusText(ww.Status()))
		metrics.RestAPI.NewRequest(r.Method, routePattern, status, time.Since(start))
	})
}
