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
			Name:      "request_duration_milliseconds",
			Help:      "How long it took to handle the request.",
		},
		[]string{"method", "path", "status"},
	),
}

type RestAPIExporter struct {
	total    *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

func (r *RestAPIExporter) NewRequest(method string, path string, status string, duration time.Duration) {
	const msInSecond = 1000

	labels := prometheus.Labels{
		"method": method,
		"path":   path,
		"status": status,
	}

	r.total.With(labels).Inc()
	r.duration.With(labels).Observe(msInSecond * duration.Seconds())
}
