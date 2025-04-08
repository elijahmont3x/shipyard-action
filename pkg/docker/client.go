package docker

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"bytes"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"

	"github.com/elijahmont3x/shipyard-action/pkg/log"
)

// Client wraps the Docker client functionality
type Client struct {
	client  *client.Client
	logger  *log.Logger
	network string
}

// NewClient creates a new Docker client
func NewClient(logger *log.Logger) (*Client, error) {
	dockerHost := os.Getenv("INPUT_DOCKER_HOST")
	if dockerHost == "" {
		dockerHost = "unix:///var/run/docker.sock"
	}

	cli, err := client.NewClientWithOpts(
		client.WithHost(dockerHost),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	return &Client{
		client:  cli,
		logger:  logger.WithField("component", "docker"),
		network: "shipyard",
	}, nil
}

// Setup creates a dedicated network for shipyard containers
func (c *Client) Setup(ctx context.Context) error {
	// Check if the network already exists
	networks, err := c.client.NetworkList(ctx, types.NetworkListOptions{
		Filters: filters.NewArgs(filters.Arg("name", c.network)),
	})
	if err != nil {
		return fmt.Errorf("failed to list networks: %w", err)
	}

	if len(networks) == 0 {
		c.logger.Info("Creating Docker network", "name", c.network)
		_, err := c.client.NetworkCreate(ctx, c.network, types.NetworkCreate{
			CheckDuplicate: true,
			Driver:         "bridge",
			Labels: map[string]string{
				"shipyard.managed": "true",
			},
		})
		if err != nil {
			return fmt.Errorf("failed to create network: %w", err)
		}
	} else {
		c.logger.Debug("Network already exists", "name", c.network)
	}

	return nil
}

// PullImage pulls a Docker image
func (c *Client) PullImage(ctx context.Context, image string) error {
	c.logger.Info("Pulling image", "image", image)

	reader, err := c.client.ImagePull(ctx, image, types.ImagePullOptions{})
	if err != nil {
		return fmt.Errorf("failed to pull image %s: %w", image, err)
	}
	defer reader.Close()

	// Read the output to complete the pull operation
	_, err = io.Copy(io.Discard, reader)
	if err != nil {
		return fmt.Errorf("error reading image pull response: %w", err)
	}

	c.logger.Debug("Image pull completed", "image", image)
	return nil
}

// CreateContainer creates a new container
func (c *Client) CreateContainer(ctx context.Context, name string, config ContainerConfig) (string, error) {
	c.logger.Info("Creating container", "name", name)

	// Prepare port bindings
	portBindings := nat.PortMap{}
	exposedPorts := nat.PortSet{}

	for _, portMapping := range config.Ports {
		parts := strings.Split(portMapping, ":")
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid port mapping format: %s", portMapping)
		}

		hostPort := parts[0]
		containerPort := parts[1]

		// Ensure the container port has the protocol
		if !strings.Contains(containerPort, "/") {
			containerPort = containerPort + "/tcp"
		}

		port := nat.Port(containerPort)
		exposedPorts[port] = struct{}{}
		portBindings[port] = []nat.PortBinding{
			{HostPort: hostPort},
		}
	}

	// Prepare volume mounts
	var mounts []mount.Mount
	for _, vol := range config.Volumes {
		mountType := mount.TypeVolume
		if vol.Type == "bind" {
			mountType = mount.TypeBind
		}

		mounts = append(mounts, mount.Mount{
			Type:   mountType,
			Source: vol.Source,
			Target: vol.Destination,
		})
	}

	// Prepare container labels
	labels := map[string]string{
		"shipyard.managed": "true",
		"shipyard.name":    name,
	}
	for k, v := range config.Labels {
		labels[k] = v
	}

	// Create health check if specified
	var healthcheck *container.HealthConfig
	if config.HealthCheck != nil {
		test := []string{}
		switch config.HealthCheck.Type {
		case "http":
			test = []string{
				"CMD-SHELL",
				fmt.Sprintf("curl -f http://localhost:%d%s || exit 1",
					config.HealthCheck.Port,
					config.HealthCheck.Path),
			}
		case "tcp":
			test = []string{
				"CMD-SHELL",
				fmt.Sprintf("nc -z localhost %d || exit 1",
					config.HealthCheck.Port),
			}
		case "custom":
			test = config.HealthCheck.Command
		}

		if len(test) > 0 {
			healthcheck = &container.HealthConfig{
				Test:        test,
				Interval:    time.Duration(config.HealthCheck.Interval) * time.Second,
				Timeout:     time.Duration(config.HealthCheck.Timeout) * time.Second,
				Retries:     config.HealthCheck.Retries,
				StartPeriod: time.Duration(config.HealthCheck.StartPeriod) * time.Second,
			}
		}
	}

	// Create the container
	resp, err := c.client.ContainerCreate(
		ctx,
		&container.Config{
			Image:        config.Image,
			Env:          formatEnvVars(config.Environment),
			ExposedPorts: exposedPorts,
			Labels:       labels,
			Healthcheck:  healthcheck,
		},
		&container.HostConfig{
			PortBindings: portBindings,
			Mounts:       mounts,
			RestartPolicy: container.RestartPolicy{
				Name: config.RestartPolicy,
			},
		},
		&network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{
				c.network: {},
			},
		},
		nil,
		name,
	)

	if err != nil {
		return "", fmt.Errorf("failed to create container %s: %w", name, err)
	}

	c.logger.Debug("Container created", "name", name, "id", resp.ID)
	return resp.ID, nil
}

// StartContainer starts a container by ID
func (c *Client) StartContainer(ctx context.Context, id string) error {
	c.logger.Info("Starting container", "id", id)

	if err := c.client.ContainerStart(ctx, id, types.ContainerStartOptions{}); err != nil {
		return fmt.Errorf("failed to start container %s: %w", id, err)
	}

	return nil
}

// StopContainer stops a container by ID
func (c *Client) StopContainer(ctx context.Context, id string, timeout int) error {
	c.logger.Info("Stopping container", "id", id, "timeout", timeout)

	// Create the stop options with the timeout
	stopOptions := container.StopOptions{
		Timeout: &timeout,
	}

	if err := c.client.ContainerStop(ctx, id, stopOptions); err != nil {
		return fmt.Errorf("failed to stop container %s: %w", id, err)
	}

	return nil
}

// RemoveContainer removes a container by ID
func (c *Client) RemoveContainer(ctx context.Context, id string, force bool) error {
	c.logger.Info("Removing container", "id", id, "force", force)

	if err := c.client.ContainerRemove(ctx, id, types.ContainerRemoveOptions{
		Force: force,
	}); err != nil {
		return fmt.Errorf("failed to remove container %s: %w", id, err)
	}

	return nil
}

// GetContainerByName retrieves a container by name
func (c *Client) GetContainerByName(ctx context.Context, name string) (*types.Container, error) {
	containers, err := c.client.ContainerList(ctx, types.ContainerListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("name", name)),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	if len(containers) == 0 {
		return nil, nil
	}

	return &containers[0], nil
}

// formatEnvVars converts a map to an array of KEY=VALUE strings
func formatEnvVars(env map[string]string) []string {
	var result []string
	for k, v := range env {
		result = append(result, fmt.Sprintf("%s=%s", k, v))
	}
	return result
}

// Execute executes a command in a container
func (c *Client) Execute(ctx context.Context, containerID string, cmd []string) (int, string, string, error) {
	c.logger.Debug("Executing command in container", "container", containerID, "command", strings.Join(cmd, " "))

	// Create exec configuration
	execConfig := types.ExecConfig{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	}

	// Create the exec instance
	exec, err := c.client.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return -1, "", "", fmt.Errorf("failed to create exec instance: %w", err)
	}

	// Start the exec instance and attach to it
	resp, err := c.client.ContainerExecAttach(ctx, exec.ID, types.ExecStartCheck{})
	if err != nil {
		return -1, "", "", fmt.Errorf("failed to start exec instance: %w", err)
	}
	defer resp.Close()

	// Read the output
	var stdoutBuf, stderrBuf bytes.Buffer
	_, err = stdcopy.StdCopy(&stdoutBuf, &stderrBuf, resp.Reader)
	if err != nil {
		return -1, "", "", fmt.Errorf("error reading exec output: %w", err)
	}

	// Get the exit code
	inspect, err := c.client.ContainerExecInspect(ctx, exec.ID)
	if err != nil {
		return -1, stdoutBuf.String(), stderrBuf.String(), fmt.Errorf("failed to inspect exec instance: %w", err)
	}

	return inspect.ExitCode, stdoutBuf.String(), stderrBuf.String(), nil
}

// Close closes the Docker client
func (c *Client) Close() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}
