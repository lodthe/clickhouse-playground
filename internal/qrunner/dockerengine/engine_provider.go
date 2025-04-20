package dockerengine

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/docker/cli/cli/connhelper"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	dockercli "github.com/docker/docker/client"
	"github.com/pkg/errors"
)

const DefaultDockerTimeout = 5 * time.Minute

// engineProvider simplifies communication with Docker Engine API.
type engineProvider struct {
	mainCtx context.Context
	cli     *dockercli.Client
}

func newProvider(ctx context.Context, daemonURL *string) (*engineProvider, error) {
	opts, err := getDockerEngineOpts(daemonURL)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build options for Docker client")
	}

	cli, err := dockercli.NewClientWithOpts(opts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create Docker client")
	}

	return &engineProvider{
		mainCtx: ctx,
		cli:     cli,
	}, nil
}

func getDockerEngineOpts(daemonURL *string) ([]dockercli.Opt, error) {
	opts := []dockercli.Opt{
		dockercli.WithAPIVersionNegotiation(),
		dockercli.WithTimeout(DefaultDockerTimeout),
	}

	if daemonURL == nil {
		return opts, nil
	}

	// Set 'StrictHostKeyChecking=no' to simplify startup in Docker containers.
	helper, err := connhelper.GetConnectionHelperWithSSHOpts(*daemonURL, []string{"-o", "StrictHostKeyChecking=no"})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create ssh connection")
	}
	if helper == nil {
		return nil, errors.Wrap(err, "provided daemon_url cannot be recognized by Docker lib")
	}

	httpClient := &http.Client{
		Transport: &http.Transport{
			DialContext: helper.Dialer,
		},
	}

	opts = append(opts,
		dockercli.WithHTTPClient(httpClient),
		dockercli.WithHost(helper.Host),
		dockercli.WithDialContext(helper.Dialer),
	)

	return opts, nil
}

func (p *engineProvider) ping(ctx context.Context) error {
	_, err := p.cli.Ping(ctx)

	return err
}

func (p *engineProvider) ownershipLabelFilter() (key, value string) {
	return "label", LabelOwnership
}

func (p *engineProvider) pullImage(ctx context.Context, imageTag string) (io.ReadCloser, error) {
	return p.cli.ImagePull(ctx, imageTag, image.PullOptions{})
}

func (p *engineProvider) addImageTag(ctx context.Context, existingImageTag, newImageTag string) error {
	return p.cli.ImageTag(ctx, existingImageTag, newImageTag)
}

func (p *engineProvider) getImageByID(ctx context.Context, id string) (image.InspectResponse, error) {
	inspect, _, err := p.cli.ImageInspectWithRaw(ctx, id)

	return inspect, err
}

// getImages returns existing images.
// If filterChp is true, only created by the playground images are returned.s
func (p *engineProvider) getImages(ctx context.Context, filterChp bool) ([]image.Summary, error) {
	images, err := p.cli.ImageList(ctx, image.ListOptions{
		All: true,
	})

	if err != nil || !filterChp {
		return images, err
	}

	for i := 0; i < len(images); i++ {
		var matched bool
		for _, tag := range images[i].RepoTags {
			if IsPlaygroundImageName(tag) {
				matched = true
				break
			}
		}

		// If it's not chp-image, swap if with the last element and pop it in O(1).
		if !matched {
			images[i] = images[len(images)-1]
			images = images[:len(images)-1]
			i--
		}
	}

	return images, nil
}

func (p *engineProvider) removeImage(ctx context.Context, tag string, pruneChildren bool) ([]image.DeleteResponse, error) {
	return p.cli.ImageRemove(ctx, tag, image.RemoveOptions{
		PruneChildren: pruneChildren,
	})
}

func (p *engineProvider) createContainer(ctx context.Context, config *container.Config, hostConfig *container.HostConfig) (container.CreateResponse, error) {
	return p.cli.ContainerCreate(ctx, config, hostConfig, nil, nil, "")
}

func (p *engineProvider) startContainer(ctx context.Context, id string) error {
	return p.cli.ContainerStart(ctx, id, container.StartOptions{})
}

func (p *engineProvider) pauseContainer(ctx context.Context, id string) error {
	return p.cli.ContainerPause(ctx, id)
}

func (p *engineProvider) unpauseContainer(ctx context.Context, id string) error {
	return p.cli.ContainerUnpause(ctx, id)
}

// exec executes the given command in the container and attaches to it.
// Keep in mind that you have to close the returned response.
func (p *engineProvider) exec(ctx context.Context, containerID string, cmd []string) (types.HijackedResponse, error) {
	exec, err := p.cli.ContainerExecCreate(ctx, containerID, container.ExecOptions{
		AttachStderr: true,
		AttachStdout: true,
		Cmd:          cmd,
	})
	if err != nil {
		return types.HijackedResponse{}, errors.Wrap(err, "exec create failed")
	}

	resp, err := p.cli.ContainerExecAttach(ctx, exec.ID, container.ExecStartOptions{})
	if err != nil {
		return types.HijackedResponse{}, errors.Wrap(err, "exec attach failed")
	}

	return resp, nil
}

func (p *engineProvider) getContainers(ctx context.Context) ([]container.Summary, error) {
	return p.cli.ContainerList(ctx, container.ListOptions{
		Size:    true,
		All:     true,
		Limit:   -1,
		Filters: filters.NewArgs(filters.Arg(p.ownershipLabelFilter())),
	})
}

func (p *engineProvider) removeContainer(ctx context.Context, id string) error {
	return p.cli.ContainerRemove(ctx, id, container.RemoveOptions{
		RemoveVolumes: true,
		Force:         true,
	})
}

func (p *engineProvider) pruneContainers(ctx context.Context) (container.PruneReport, error) {
	return p.cli.ContainersPrune(ctx, filters.NewArgs(filters.Arg(p.ownershipLabelFilter())))
}
