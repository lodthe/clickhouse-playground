package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var LocalDockerGC = newRunnerGCExporter("local_docker")

func newRunnerGCExporter(runner string) RunnerGCExporter {
	runnerLabels := prometheus.Labels{
		"runner": runner,
	}

	return RunnerGCExporter{
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
	}
}

type RunnerGCExporter struct {
	duration       *prometheus.HistogramVec
	objCollected   *prometheus.CounterVec
	spaceReclaimed *prometheus.CounterVec
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
