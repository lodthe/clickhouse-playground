package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

func NewPipelineExporter(runnerType, runnerName string) *PipelineExporter {
	runnerLabels := prometheus.Labels{
		"runner_type": runnerType,
		"runner_name": runnerName,
	}

	return &PipelineExporter{
		duration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace:   "runner",
				Name:        "pipeline_step_duration_seconds",
				Help:        "How long it took to process a runner pipeline step, partitioned by step name, database version and status (success or failure).",
				ConstLabels: runnerLabels,
				Buckets:     defaultPipelineBuckets,
			},
			[]string{"step", "version", "status"},
		),
	}
}

type PipelineExporter struct {
	duration *prometheus.HistogramVec
}

func (r *PipelineExporter) observe(step string, succeed bool, version string, startedAt time.Time) {
	status := "success"
	if !succeed {
		status = "failure"
	}

	r.duration.
		With(prometheus.Labels{
			"step":    step,
			"version": version,
			"status":  status,
		}).
		Observe(time.Since(startedAt).Seconds())
}

func (r *PipelineExporter) PullExistedImage(succeed bool, version string, startedAt time.Time) {
	r.observe("pull_existed_image", succeed, version, startedAt)
}

func (r *PipelineExporter) PullNewImage(succeed bool, version string, startedAt time.Time) {
	r.observe("pull_new_image", succeed, version, startedAt)
}

func (r *PipelineExporter) CreateContainer(succeed bool, version string, startedAt time.Time) {
	r.observe("create_container", succeed, version, startedAt)
}

func (r *PipelineExporter) ExecCommand(succeed bool, version string, startedAt time.Time) {
	r.observe("exec_command", succeed, version, startedAt)
}

func (r *PipelineExporter) RunQuery(succeed bool, version string, startedAt time.Time) {
	r.observe("run_query", succeed, version, startedAt)
}

func (r *PipelineExporter) RemoveContainer(succeed bool, version string, startedAt time.Time) {
	r.observe("remove_container", succeed, version, startedAt)
}
