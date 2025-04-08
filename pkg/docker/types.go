package docker

// ContainerConfig represents the configuration for creating a container
type ContainerConfig struct {
	Image         string
	Environment   map[string]string
	Ports         []string
	Volumes       []Volume
	Labels        map[string]string
	HealthCheck   *HealthCheck
	RestartPolicy string
}

// Volume represents a Docker volume configuration
type Volume struct {
	Source      string
	Destination string
	Type        string
}

// HealthCheck represents a Docker health check configuration
type HealthCheck struct {
	Type        string
	Path        string
	Port        int
	Command     []string
	Interval    int
	Timeout     int
	Retries     int
	StartPeriod int
}
