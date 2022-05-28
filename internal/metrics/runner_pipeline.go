package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var DockerEnginePipeline = DockerEnginePipelineExporter{
	duration: promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "runner",
			Name:      "pipeline_step_duration_seconds",
			Help:      "How long it took to process a runner pipeline step, partitioned by step name, database version and status (success or failure).",
			ConstLabels: prometheus.Labels{
				"runner": "docker_engine",
			},
			Buckets: []float64{.01, .05, .1, .25, .5, 1, 2.5, 5, 10, 15, 30},
		},
		[]string{"step", "version", "status"},
	),
}

type DockerEnginePipelineExporter struct {
	duration *prometheus.HistogramVec
}

func (r *DockerEnginePipelineExporter) observe(step string, succeed bool, version string, startedAt time.Time) {
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

func (r *DockerEnginePipelineExporter) PullExistedImage(succeed bool, version string, startedAt time.Time) {
	r.observe("pull_existed_image", succeed, version, startedAt)
}

func (r *DockerEnginePipelineExporter) PullNewImage(succeed bool, version string, startedAt time.Time) {
	r.observe("pull_new_image", succeed, version, startedAt)
}

func (r *DockerEnginePipelineExporter) CreateContainer(succeed bool, version string, startedAt time.Time) {
	r.observe("create_container", succeed, version, startedAt)
}

func (r *DockerEnginePipelineExporter) ExecCommand(succeed bool, version string, startedAt time.Time) {
	r.observe("exec_command", succeed, version, startedAt)
}

func (r *DockerEnginePipelineExporter) RunQuery(succeed bool, version string, startedAt time.Time) {
	r.observe("run_query", succeed, version, startedAt)
}

func (r *DockerEnginePipelineExporter) RemoveContainer(succeed bool, version string, startedAt time.Time) {
	r.observe("remove_container", succeed, version, startedAt)
}
