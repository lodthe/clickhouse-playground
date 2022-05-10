package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var LocalDockerPipeline = LocalDockerPipelineExporter{
	duration: promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "runner",
			Name:      "pipeline_step_duration_seconds",
			Help:      "How long it took to process a runner pipeline step, partitioned by step name, database version and status (success or failure).",
			ConstLabels: prometheus.Labels{
				"runner": "local_docker",
			},
			Buckets: []float64{.01, .05, .1, .25, .5, 1, 2.5, 5, 10, 15, 30},
		},
		[]string{"step", "version", "status"},
	),
}

type LocalDockerPipelineExporter struct {
	duration *prometheus.HistogramVec
}

func (r *LocalDockerPipelineExporter) observe(step string, succeed bool, version string, startedAt time.Time) {
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

func (r *LocalDockerPipelineExporter) PullExistedImage(succeed bool, version string, startedAt time.Time) {
	r.observe("pull_existed_image", succeed, version, startedAt)
}

func (r *LocalDockerPipelineExporter) PullNewImage(succeed bool, version string, startedAt time.Time) {
	r.observe("pull_new_image", succeed, version, startedAt)
}

func (r *LocalDockerPipelineExporter) CreateContainer(succeed bool, version string, startedAt time.Time) {
	r.observe("create_container", succeed, version, startedAt)
}

func (r *LocalDockerPipelineExporter) ExecCommand(succeed bool, version string, startedAt time.Time) {
	r.observe("exec_command", succeed, version, startedAt)
}

func (r *LocalDockerPipelineExporter) RunQuery(succeed bool, version string, startedAt time.Time) {
	r.observe("run_query", succeed, version, startedAt)
}

func (r *LocalDockerPipelineExporter) KillContainer(succeed bool, version string, startedAt time.Time) {
	r.observe("kill_container", succeed, version, startedAt)
}
