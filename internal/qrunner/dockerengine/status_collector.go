package dockerengine

import (
	"context"
	"time"

	"clickhouse-playground/internal/metrics"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

type statusCollector struct {
	ctx context.Context

	logger zerolog.Logger

	engine *engineProvider
	metr   *metrics.RunnerStatusExporter

	repository string
	frequency  time.Duration
}

func newStatusCollector(ctx context.Context, logger zerolog.Logger, repo string, collectFrequency time.Duration, engine *engineProvider, metr *metrics.RunnerStatusExporter) *statusCollector {
	return &statusCollector{
		ctx:        ctx,
		logger:     logger,
		engine:     engine,
		metr:       metr,
		repository: repo,
		frequency:  collectFrequency,
	}
}

func (s *statusCollector) start() {
	s.logger.Info().Dur("trigger_frequency", s.frequency).Msg("status collector has been started")
	defer s.logger.Info().Msg("status collector has been finished")

	collect := func() {
		err := s.collect()
		if err != nil {
			s.logger.Err(err).Msg("failed to collect runner status")
		}
	}

	collect()

	t := time.NewTicker(s.frequency)

	for {
		select {
		case <-s.ctx.Done():
			return

		case <-t.C:
		}

		collect()
	}
}

func (s *statusCollector) collect() error {
	imgCount, imgSpace, err := s.collectImages()
	if err != nil {
		return errors.Wrap(err, "failed to get check images status")
	}

	s.metr.UpdateImageStatus(imgCount, imgSpace)

	contCount, contSpace, err := s.collectContainers()
	if err != nil {
		return errors.Wrap(err, "failed to check containers status")
	}

	s.metr.UpdateContainerStatus(contCount, contSpace)

	return nil
}

func (s *statusCollector) collectImages() (count uint, space uint64, err error) {
	images, err := s.engine.getImages(s.ctx, s.repository, true)
	if err != nil {
		return 0, 0, errors.Wrap(err, "failed to get images from engine")
	}

	for _, img := range images {
		count++
		space += uint64(img.Size)
	}

	return count, space, nil
}

func (s *statusCollector) collectContainers() (count uint, space uint64, err error) {
	containers, err := s.engine.getContainers(s.ctx)
	if err != nil {
		return 0, 0, errors.Wrap(err, "failed to get containers from engine")
	}

	for _, c := range containers {
		count++
		space += uint64(c.SizeRw)
	}

	return count, space, nil
}
