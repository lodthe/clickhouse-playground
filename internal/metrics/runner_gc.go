package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type RunnerGCExporter struct {
	duration         *prometheus.HistogramVec
	objCollected     *prometheus.CounterVec
	spaceReclaimed   *prometheus.CounterVec
	pausedContainers prometheus.Gauge
}

func NewRunnerGCExporter(runnerType, runnerName string) *RunnerGCExporter {
	runnerLabels := prometheus.Labels{
		"runner_type": runnerType,
		"runner_name": runnerName,
	}

	return &RunnerGCExporter{
		duration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace:   "runner",
				Name:        "gc_duration_seconds",
				Help:        "How long it took to collect containers and images.",
				ConstLabels: runnerLabels,
				Buckets:     prometheus.DefBuckets,
			},
			[]string{"object"},
		),
		objCollected: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace:   "runner",
				Name:        "gc_objects_collected_total",
				Help:        "How many objects have been collected.",
				ConstLabels: runnerLabels,
			},
			[]string{"object"},
		),
		spaceReclaimed: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace:   "runner",
				Name:        "gc_space_reclaimed_bytes",
				Help:        "Disk space that have been reclaimed.",
				ConstLabels: runnerLabels,
			},
			[]string{"object"},
		),
		pausedContainers: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace:   "runner",
				Name:        "paused_containers",
				Help:        "Number of prewarmed containers at the moment.",
				ConstLabels: runnerLabels,
			},
		),
	}
}

func (r *RunnerGCExporter) objectsCollected(object string, count uint, spaceReclaimed uint64, startedAt time.Time) {
	lbl := prometheus.Labels{"object": object}

	r.duration.With(lbl).Observe(time.Since(startedAt).Seconds())
	r.objCollected.With(lbl).Add(float64(count))
	r.spaceReclaimed.With(lbl).Add(float64(spaceReclaimed))
}

func (r *RunnerGCExporter) ContainersCollected(count uint, spaceReclaimed uint64, startedAt time.Time) {
	r.objectsCollected("container", count, spaceReclaimed, startedAt)
}

func (r *RunnerGCExporter) ImagesCollected(count uint, spaceReclaimed uint64, startedAt time.Time) {
	r.objectsCollected("image", count, spaceReclaimed, startedAt)
}

func (r *RunnerGCExporter) ReportPausedContainers(count uint) {
	r.pausedContainers.Set(float64(count))
}
