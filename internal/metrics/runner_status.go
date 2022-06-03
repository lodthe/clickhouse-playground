package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

func NewRunnerStatusExporter(runnerType, runnerName string) *RunnerStatusExporter {
	runnerLabels := prometheus.Labels{
		"runner_type": runnerType,
		"runner_name": runnerName,
	}

	return &RunnerStatusExporter{
		objects: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace:   "runner",
				Name:        "status_existing_objects_count",
				Help:        "Number of existing objects.",
				ConstLabels: runnerLabels,
			},
			[]string{"object"},
		),
		spaceConsumption: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace:   "runner",
				Name:        "status_space_consumption_bytes",
				Help:        "How much disk space do existing objects consume.",
				ConstLabels: runnerLabels,
			},
			[]string{"object"},
		),
	}
}

type RunnerStatusExporter struct {
	objects          *prometheus.GaugeVec
	spaceConsumption *prometheus.GaugeVec
}

func (r *RunnerStatusExporter) set(object string, count uint, spaceConsumption uint64) {
	lbl := prometheus.Labels{"object": object}

	r.objects.With(lbl).Set(float64(count))
	r.spaceConsumption.With(lbl).Set(float64(spaceConsumption))
}

func (r *RunnerStatusExporter) UpdateContainerStatus(count uint, spaceConsumption uint64) {
	r.set("container", count, spaceConsumption)
}

func (r *RunnerStatusExporter) UpdateImageStatus(count uint, spaceConsumption uint64) {
	r.set("image", count, spaceConsumption)
}
