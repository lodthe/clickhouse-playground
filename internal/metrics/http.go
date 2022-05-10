package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var RestAPI = RestAPIExporter{
	total: promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "http",
			Name:      "requests_total",
			Help:      "How many HTTP requests were handled.",
		},
		[]string{"method", "path", "status"},
	),
	duration: promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "http",
			Name:      "request_duration_seconds",
			Help:      "How long it took to handle the request.",
			Buckets:   []float64{.005, .01, .05, .1, .25, .5, 1, 2.5, 5, 10, 15, 30},
		},
		[]string{"method", "path", "status"},
	),
}

type RestAPIExporter struct {
	total    *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

func (r *RestAPIExporter) NewRequest(method string, path string, status string, duration time.Duration) {
	labels := prometheus.Labels{
		"method": method,
		"path":   path,
		"status": status,
	}

	r.total.With(labels).Inc()
	r.duration.With(labels).Observe(duration.Seconds())
}
