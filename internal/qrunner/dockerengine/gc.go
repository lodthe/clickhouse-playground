package dockerengine

import (
	"context"
	"sort"
	"time"

	"github.com/lodthe/clickhouse-playground/internal/metrics"

	"github.com/docker/docker/api/types/image"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

const PausedContainersMaxTTL = 24 * time.Hour

type garbageCollector struct {
	ctx context.Context

	logger zerolog.Logger

	cfg *GCConfig

	engine *engineProvider
	metr   *metrics.RunnerGCExporter
}

func newGarbageCollector(ctx context.Context, logger zerolog.Logger, cfg *GCConfig, engine *engineProvider, metr *metrics.RunnerGCExporter) *garbageCollector {
	return &garbageCollector{
		ctx:    ctx,
		logger: logger,
		cfg:    cfg,
		engine: engine,
		metr:   metr,
	}
}

func (g *garbageCollector) isStopped() bool {
	select {
	case <-g.ctx.Done():
		return true

	default:
		return false
	}
}

func (g *garbageCollector) start() {
	if g.cfg == nil {
		g.logger.Info().Msg("garbage collector is disabled due to a missed configuration")
		return
	}

	g.logger.Info().Dur("trigger_frequency", g.cfg.TriggerFrequency).Msg("gc has been started")
	defer g.logger.Info().Msg("gc has been finished")

	trigger := func() {
		err := g.trigger()
		if err != nil {
			g.logger.Err(err).Msg("gc trigger failed")
		}
	}

	trigger()

	t := time.NewTicker(g.cfg.TriggerFrequency)

	for {
		select {
		case <-g.ctx.Done():
			return

		case <-t.C:
		}

		trigger()
	}
}

func (g *garbageCollector) trigger() (err error) {
	if g.isStopped() {
		return nil
	}

	_, _, err = g.collectContainers()
	if err != nil {
		return errors.Wrap(err, "containers gc failed")
	}

	if g.isStopped() {
		return nil
	}

	_, _, err = g.collectImages()
	if err != nil {
		return errors.Wrap(err, "images gc failed")
	}

	g.logger.Debug().Msg("gc finished")

	return nil
}

// collectContainers prunes exited containers and force removes hanged up containers.
// A container is hanged up if it has been alive at least for GCConfig.ContainerTTL.
func (g *garbageCollector) collectContainers() (count uint, spaceReclaimed uint64, err error) {
	startedAt := time.Now()
	defer func() {
		g.metr.ContainersCollected(count, spaceReclaimed, startedAt)
	}()

	out, err := g.engine.pruneContainers(g.ctx)
	if err != nil {
		return 0, 0, errors.Wrap(err, "failed to prune stopped containers")
	}

	count += uint(len(out.ContainersDeleted))
	spaceReclaimed += out.SpaceReclaimed

	if g.cfg.ContainerTTL == nil {
		return count, spaceReclaimed, nil
	}

	// Find hanged up containers and force remove them.
	containers, err := g.engine.getContainers(g.ctx)
	if err != nil {
		return count, spaceReclaimed, errors.Wrap(err, "failed to list containers")
	}

	var pausedContainers uint
	for _, c := range containers {
		createdAt := time.Unix(c.Created, 0)
		deadline := createdAt.Add(*g.cfg.ContainerTTL)
		if time.Now().Before(deadline) {
			continue
		}

		if c.State == "paused" {
			pausedContainers++
			if time.Since(createdAt) < PausedContainersMaxTTL {
				continue
			}
		}

		err = g.engine.removeContainer(g.ctx, c.ID)
		if err != nil {
			g.logger.Error().Err(err).Str("container_id", c.ID).Msg("containers gc failed to remove container")
			continue
		}

		g.logger.Debug().Str("container_id", c.ID).Msg("container has been force removed")

		count++
		spaceReclaimed += uint64(c.SizeRw)
	}

	g.metr.ReportPausedContainers(pausedContainers)

	return count, spaceReclaimed, nil
}

// collectImages frees the disk by removing most recently tagged images.
// If there are at least GCConfig.ImageGCCountThreshold downloaded chp images, it leaves GCConfig.ImageBufferSize
// least recently tagged images and removes the others.
func (g *garbageCollector) collectImages() (count uint, spaceReclaimed uint64, err error) {
	startedAt := time.Now()
	defer func() {
		g.metr.ContainersCollected(count, spaceReclaimed, startedAt)
	}()

	images, err := g.engine.getImages(g.ctx, true)
	if err != nil {
		return 0, 0, errors.Wrap(err, "failed to list images")
	}

	if len(images) < int(*g.cfg.ImageGCCountThreshold) {
		return 0, 0, nil
	}

	detailed := make([]image.InspectResponse, 0, len(images))
	for _, c := range images {
		inspect, err := g.engine.getImageByID(g.ctx, c.ID)
		if err != nil {
			g.logger.Err(err).Str("image_id", c.ID).Msg("docker image inspect failed")
			continue
		}

		detailed = append(detailed, inspect)
	}

	// Drop N least recently tagged images.
	sort.Slice(detailed, func(i, j int) bool {
		return detailed[i].Metadata.LastTagTime.Before(detailed[j].Metadata.LastTagTime)
	})

	if len(detailed) > int(g.cfg.ImageBufferSize) {
		count, spaceReclaimed = g.removeImages(detailed[int(g.cfg.ImageBufferSize):])
	}

	return count, spaceReclaimed, nil
}

// removeImages deletes all tags of the provided images.
func (g *garbageCollector) removeImages(images []image.InspectResponse) (count uint, spaceReclaimed uint64) {
	for _, img := range images {
		ok := true
		for _, tag := range img.RepoTags {
			_, err := g.engine.removeImage(g.ctx, tag, true)
			if err != nil {
				g.logger.Err(err).Str("image_id", img.ID).Msg("failed to delete image tag")
				ok = false

				continue
			}
		}

		if !ok {
			continue
		}

		g.logger.Debug().Str("id", img.ID).Strs("tags", img.RepoTags).Msg("image has been removed")

		count++
		spaceReclaimed += uint64(img.Size)
	}

	return count, spaceReclaimed
}
