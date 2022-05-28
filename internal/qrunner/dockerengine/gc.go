package dockerengine

import (
	"context"
	"sort"
	"time"

	"clickhouse-playground/internal/metrics"
	"clickhouse-playground/internal/qrunner"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	dockercli "github.com/docker/docker/client"
	"github.com/pkg/errors"
	zlog "github.com/rs/zerolog/log"
)

type garbageCollector struct {
	ctx        context.Context
	cfg        *GCConfig
	repository string

	cli  *dockercli.Client
	metr *metrics.RunnerGCExporter
}

func newGarbageCollector(ctx context.Context, cfg *GCConfig, repository string, cli *dockercli.Client, metr *metrics.RunnerGCExporter) *garbageCollector {
	return &garbageCollector{
		ctx:        ctx,
		cfg:        cfg,
		repository: repository,
		cli:        cli,
		metr:       metr,
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
		zlog.Info().Msg("garbage collector is disabled due to a missed configuration")
		return
	}

	zlog.Info().Dur("trigger_frequency", g.cfg.TriggerFrequency).Msg("dockerengine gc has been started")
	defer zlog.Info().Msg("dockerengine gc has been finished")

	trigger := func() {
		err := g.trigger()
		if err != nil {
			zlog.Err(err).Msg("dockerengine gc trigger failed")
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

	zlog.Debug().Msg("gc finished")

	return nil
}

// collectContainers prunes exited containers and force removes hanged up containers.
// A container is hanged up if it has been alive at least for GCConfig.ContainerTTL.
func (g *garbageCollector) collectContainers() (count uint, spaceReclaimed uint64, err error) {
	startedAt := time.Now()
	defer func() {
		g.metr.ContainersCollected(count, spaceReclaimed, startedAt)
	}()

	out, err := g.cli.ContainersPrune(g.ctx, filters.NewArgs(filters.Arg("label", qrunner.LabelOwnership)))
	if err != nil {
		return 0, 0, errors.Wrap(err, "failed to prune stopped containers")
	}

	count += uint(len(out.ContainersDeleted))
	spaceReclaimed += out.SpaceReclaimed

	if g.cfg.ContainerTTL == nil {
		return count, spaceReclaimed, nil
	}

	// Find hanged up containers and force remove them.
	containers, err := g.cli.ContainerList(g.ctx, types.ContainerListOptions{
		Size:    true,
		All:     true,
		Limit:   -1,
		Filters: filters.NewArgs(filters.Arg("label", qrunner.LabelOwnership)),
	})
	if err != nil {
		return count, spaceReclaimed, errors.Wrap(err, "failed to list containers")
	}

	for _, c := range containers {
		deadline := time.Unix(c.Created, 0).Add(*g.cfg.ContainerTTL)
		if time.Now().Before(deadline) {
			continue
		}

		err = g.forceRemoveContainer(c.ID)
		if err != nil {
			zlog.Error().Err(err).Str("container_id", c.ID).Msg("containers gc failed to remove container")
			continue
		}

		count++
		spaceReclaimed += uint64(c.SizeRw)
	}

	return count, spaceReclaimed, nil
}

// collectImages frees the disk by removing most recently tagged images.
// If there are at least GCConfig.ImageGCCountThreshold downloaded chp images, it leaves GCConfig.ImageBufferSize
// least recently tagged images and removes the others.
func (g *garbageCollector) collectImages() (count uint, spaceReclaimed uint64, err error) {
	if g.cfg.ImageGCCountThreshold == nil {
		return 0, 0, nil
	}

	startedAt := time.Now()
	defer func() {
		g.metr.ContainersCollected(count, spaceReclaimed, startedAt)
	}()

	images, err := g.cli.ImageList(g.ctx, types.ImageListOptions{})
	if err != nil {
		return 0, 0, errors.Wrap(err, "failed to list images")
	}

	// Find all images with chp tags.
	var candidates []types.ImageSummary
	for _, img := range images {
		var matched bool
		for _, tag := range img.RepoTags {
			if qrunner.IsPlaygroundImageName(tag, g.repository) {
				matched = true
				break
			}
		}

		if !matched {
			continue
		}

		candidates = append(candidates, img)
	}

	if len(candidates) < int(*g.cfg.ImageGCCountThreshold) {
		return 0, 0, nil
	}

	detailed := make([]types.ImageInspect, 0, len(candidates))
	for _, c := range candidates {
		inspect, _, err := g.cli.ImageInspectWithRaw(g.ctx, c.ID)
		if err != nil {
			zlog.Err(err).Str("image_id", c.ID).Msg("docker image inspect failed")
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
func (g *garbageCollector) removeImages(images []types.ImageInspect) (count uint, spaceReclaimed uint64) {
	for _, img := range images {
		ok := true
		for _, tag := range img.RepoTags {
			_, err := g.cli.ImageRemove(g.ctx, tag, types.ImageRemoveOptions{
				PruneChildren: true,
			})
			if err != nil {
				zlog.Err(err).Str("image_id", img.ID).Msg("failed to delete image tag")
				ok = false

				continue
			}
		}

		if !ok {
			continue
		}

		zlog.Debug().Str("id", img.ID).Strs("tags", img.RepoTags).Msg("image has been removed")

		count++
		spaceReclaimed += uint64(img.Size)
	}

	return count, spaceReclaimed
}

func (g *garbageCollector) forceRemoveContainer(id string) (err error) {
	err = g.cli.ContainerRemove(g.ctx, id, types.ContainerRemoveOptions{
		RemoveVolumes: true,
		Force:         true,
	})
	if err != nil {
		return err
	}

	zlog.Debug().Str("container_id", id).Msg("container has been force removed")

	return nil
}
