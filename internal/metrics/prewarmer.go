package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type PrewarmerExporter struct {
	fetchesTotal         *prometheus.CounterVec
	containersSetUpdates *prometheus.CounterVec
}

func NewPrewarmerExporter() *PrewarmerExporter {
	return &PrewarmerExporter{
		fetchesTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "prewarmer",
				Name:      "fetch_requests_total",
				Help:      "How many fetch requests were dispatched.",
			},
			[]string{"status"},
		),
		containersSetUpdates: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "prewarmer",
				Name:      "containers_set_updates_total",
				Help:      "How many changes of containers set are done.",
			},
			[]string{"action"},
		),
	}
}

func (r *PrewarmerExporter) FetchHit() {
	r.observeFetch("hit")
}

func (r *PrewarmerExporter) FetchMiss() {
	r.observeFetch("miss")
}

func (r *PrewarmerExporter) observeFetch(status string) {
	r.fetchesTotal.
		With(prometheus.Labels{
			"status": status,
		}).
		Inc()
}

func (r *PrewarmerExporter) AddContainer() {
	r.observeUpdate("add")
}

func (r *PrewarmerExporter) FetchContainer() {
	r.observeUpdate("fetch")
}

func (r *PrewarmerExporter) EjectContainer() {
	r.observeUpdate("eject")
}

func (r *PrewarmerExporter) observeUpdate(action string) {
	r.containersSetUpdates.
		With(prometheus.Labels{
			"action": action,
		}).
		Inc()
}
