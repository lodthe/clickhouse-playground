package dockerengine

import (
	"context"
	"io"

	"clickhouse-playground/internal/qrunner"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	dockercli "github.com/docker/docker/client"
	"github.com/pkg/errors"
)

// engineProvider simplifies communication with Docker Engine API.
type engineProvider struct {
	mainCtx context.Context
	cli     *dockercli.Client
}

func newProvider(ctx context.Context, cli *dockercli.Client) *engineProvider {
	return &engineProvider{
		mainCtx: ctx,
		cli:     cli,
	}
}

func (p *engineProvider) ownershipLabelFilter() (key, value string) {
	return "label", qrunner.LabelOwnership
}

func (p *engineProvider) pullImage(ctx context.Context, name string) (io.ReadCloser, error) {
	return p.cli.ImagePull(ctx, name, types.ImagePullOptions{})
}

func (p *engineProvider) addImageTag(ctx context.Context, existingImageTag, newImageTag string) error {
	return p.cli.ImageTag(ctx, existingImageTag, newImageTag)
}

func (p *engineProvider) getImageByID(ctx context.Context, name string) (types.ImageInspect, error) {
	inspect, _, err := p.cli.ImageInspectWithRaw(ctx, name)

	return inspect, err
}

func (p *engineProvider) getImages(ctx context.Context) ([]types.ImageSummary, error) {
	return p.cli.ImageList(ctx, types.ImageListOptions{
		All: true,
	})
}

func (p *engineProvider) removeImage(ctx context.Context, tag string, pruneChildren bool) ([]types.ImageDeleteResponseItem, error) {
	return p.cli.ImageRemove(ctx, tag, types.ImageRemoveOptions{
		PruneChildren: pruneChildren,
	})
}

func (p *engineProvider) createContainer(ctx context.Context, config *container.Config, hostConfig *container.HostConfig) (container.ContainerCreateCreatedBody, error) {
	return p.cli.ContainerCreate(ctx, config, hostConfig, nil, nil, "")
}

func (p *engineProvider) startContainer(ctx context.Context, id string) error {
	return p.cli.ContainerStart(ctx, id, types.ContainerStartOptions{})
}

// exec executes the given command in the container and attaches to it.
// Keep in mind that you have to close the returned response.
func (p *engineProvider) exec(ctx context.Context, containerID string, cmd []string) (types.HijackedResponse, error) {
	exec, err := p.cli.ContainerExecCreate(ctx, containerID, types.ExecConfig{
		AttachStderr: true,
		AttachStdout: true,
		Cmd:          cmd,
	})
	if err != nil {
		return types.HijackedResponse{}, errors.Wrap(err, "exec create failed")
	}

	resp, err := p.cli.ContainerExecAttach(ctx, exec.ID, types.ExecStartCheck{})
	if err != nil {
		return types.HijackedResponse{}, errors.Wrap(err, "exec attach failed")
	}

	return resp, nil
}

func (p *engineProvider) getContainers(ctx context.Context) ([]types.Container, error) {
	return p.cli.ContainerList(ctx, types.ContainerListOptions{
		Size:    true,
		All:     true,
		Limit:   -1,
		Filters: filters.NewArgs(filters.Arg(p.ownershipLabelFilter())),
	})
}

func (p *engineProvider) removeContainer(ctx context.Context, id string, force bool) error {
	return p.cli.ContainerRemove(ctx, id, types.ContainerRemoveOptions{
		RemoveVolumes: true,
		Force:         force,
	})
}

func (p *engineProvider) pruneContainers(ctx context.Context) (types.ContainersPruneReport, error) {
	return p.cli.ContainersPrune(ctx, filters.NewArgs(filters.Arg(p.ownershipLabelFilter())))
}
