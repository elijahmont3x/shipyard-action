package deployment

import (
	"context"
	"fmt"
	"time"

	"github.com/elijahmont3x/shipyard-action/pkg/config"
	"github.com/elijahmont3x/shipyard-action/pkg/docker"
	"github.com/elijahmont3x/shipyard-action/pkg/health"
	"github.com/elijahmont3x/shipyard-action/pkg/log"
	"github.com/elijahmont3x/shipyard-action/pkg/proxy"
	"github.com/elijahmont3x/shipyard-action/pkg/ssl"
)

// Manager coordinates the deployment process
type Manager struct {
	logger      *log.Logger
	config      *config.Config
	docker      *docker.Client
	health      *health.Checker
	proxy       *proxy.Manager
	ssl         *ssl.Manager
	scanner     *docker.SecurityScanner
	deployments map[string]*Deployment
}

// Deployment tracks the state of a deployed service or app
type Deployment struct {
	Name          string
	ContainerID   string
	Image         string
	CreatedAt     time.Time
	ServiceType   string // "service" or "app"
	DependsOn     []string
	HealthCheck   *config.HealthCheck
	Volumes       []config.Volume
	Environment   map[string]string
	Ports         []string
	RestartPolicy string
}

// NewManager creates a new deployment manager
func NewManager(cfg *config.Config, logger *log.Logger) (*Manager, error) {
	// Create Docker client
	dockerClient, err := docker.NewClient(logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	// Create health checker
	healthChecker := health.NewChecker(logger)

	// Create proxy manager
	proxyManager := proxy.NewManager(logger, dockerClient, cfg)

	// Create SSL manager
	sslManager := ssl.NewManager(logger, cfg)

	// Create security scanner
	securityScanner := docker.NewSecurityScanner(logger)

	return &Manager{
		logger:      logger.WithField("component", "deployment"),
		config:      cfg,
		docker:      dockerClient,
		health:      healthChecker,
		proxy:       proxyManager,
		ssl:         sslManager,
		scanner:     securityScanner,
		deployments: make(map[string]*Deployment),
	}, nil
}

// Deploy executes the deployment process
func (m *Manager) Deploy(ctx context.Context) error {
	m.logger.Info("Starting deployment process")

	// Apply timeout if specified
	if m.config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(m.config.Timeout)*time.Minute)
		defer cancel()
	}

	// Setup Docker network
	if err := m.docker.Setup(ctx); err != nil {
		return fmt.Errorf("failed to setup Docker network: %w", err)
	}

	// Setup SSL certificates if enabled
	if m.config.SSL.Enabled {
		if err := m.ssl.Setup(ctx); err != nil {
			return fmt.Errorf("failed to setup SSL: %w", err)
		}
	}

	// Deploy persistent services
	if err := m.deployServices(ctx); err != nil {
		return fmt.Errorf("failed to deploy services: %w", err)
	}

	// Deploy applications
	if err := m.deployApps(ctx); err != nil {
		return fmt.Errorf("failed to deploy apps: %w", err)
	}

	// Setup proxy with deployed apps
	if err := m.proxy.Setup(ctx); err != nil {
		return fmt.Errorf("failed to setup proxy: %w", err)
	}

	m.logger.Info("Deployment completed successfully")
	return nil
}

// Rollback performs a rollback of the deployment
func (m *Manager) Rollback(ctx context.Context) error {
	m.logger.Info("Starting rollback process")

	// First, clean up the proxy
	if err := m.proxy.Cleanup(ctx); err != nil {
		m.logger.Error("Failed to clean up proxy", "error", err)
	}

	// Rollback in reverse deployment order (apps first, then services)
	var errors []error

	// Rollback apps
	for _, app := range m.config.Apps {
		if deployment, exists := m.deployments[app.Name]; exists {
			if err := m.rollbackDeployment(ctx, deployment); err != nil {
				errors = append(errors, err)
				m.logger.Error("Failed to rollback app", "name", app.Name, "error", err)
			}
		}
	}

	// Rollback services in reverse dependency order
	reversedServices := reverseServices(m.config.Services)
	for _, service := range reversedServices {
		if deployment, exists := m.deployments[service.Name]; exists {
			if err := m.rollbackDeployment(ctx, deployment); err != nil {
				errors = append(errors, err)
				m.logger.Error("Failed to rollback service", "name", service.Name, "error", err)
			}
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("rollback completed with %d errors", len(errors))
	}

	m.logger.Info("Rollback completed successfully")
	return nil
}

// deployServices deploys all services in dependency order
func (m *Manager) deployServices(ctx context.Context) error {
	m.logger.Info("Deploying services")

	// Get services in dependency order
	orderedServices, err := resolveServiceDependencies(m.config.Services)
	if err != nil {
		return fmt.Errorf("failed to resolve service dependencies: %w", err)
	}

	// Deploy each service
	for _, service := range orderedServices {
		// Skip if already deployed
		if _, exists := m.deployments[service.Name]; exists {
			continue
		}

		// Check if all dependencies are deployed
		if !m.areDependenciesDeployed(service.DependsOn) {
			return fmt.Errorf("dependencies for service %s are not yet deployed", service.Name)
		}

		// Deploy the service
		if err := m.deployService(ctx, service); err != nil {
			return fmt.Errorf("failed to deploy service %s: %w", service.Name, err)
		}
	}

	return nil
}

// deployApps deploys all applications in dependency order
func (m *Manager) deployApps(ctx context.Context) error {
	m.logger.Info("Deploying applications")

	// Get apps in dependency order
	orderedApps, err := resolveAppDependencies(m.config.Apps)
	if err != nil {
		return fmt.Errorf("failed to resolve app dependencies: %w", err)
	}

	// Deploy each app
	for _, app := range orderedApps {
		// Skip if already deployed
		if _, exists := m.deployments[app.Name]; exists {
			continue
		}

		// Check if all dependencies are deployed
		if !m.areDependenciesDeployed(app.DependsOn) {
			return fmt.Errorf("dependencies for app %s are not yet deployed", app.Name)
		}

		// Deploy the app
		if err := m.deployApp(ctx, app); err != nil {
			return fmt.Errorf("failed to deploy app %s: %w", app.Name, err)
		}
	}

	return nil
}

// deployService deploys a single service
func (m *Manager) deployService(ctx context.Context, service config.Service) error {
	m.logger.Info("Deploying service", "name", service.Name, "image", service.Image)

	// Pull the image
	if err := m.docker.PullImage(ctx, service.Image); err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}

	// Scan the image for security vulnerabilities
	scanResult, err := m.scanner.ScanImage(ctx, service.Image)
	if err != nil {
		m.logger.Warn("Failed to scan image", "name", service.Name, "error", err)
	} else if scanResult.CriticalCount > 0 {
		m.logger.Warn("Image has security vulnerabilities",
			"name", service.Name,
			"critical", scanResult.CriticalCount,
			"high", scanResult.HighCount)
	}

	// Create container config
	containerConfig := docker.ContainerConfig{
		Image:         service.Image,
		Environment:   service.Environment,
		Ports:         service.Ports,
		RestartPolicy: service.RestartPolicy,
		Labels: map[string]string{
			"shipyard.type": "service",
			"shipyard.name": service.Name,
		},
	}

	// Convert volumes
	for _, vol := range service.Volumes {
		containerConfig.Volumes = append(containerConfig.Volumes, docker.Volume{
			Source:      vol.Source,
			Destination: vol.Destination,
			Type:        vol.Type,
		})
	}

	// Create health check config if defined
	if service.HealthCheck.Type != "" {
		containerConfig.HealthCheck = &docker.HealthCheck{
			Type:        service.HealthCheck.Type,
			Path:        service.HealthCheck.Path,
			Port:        service.HealthCheck.Port,
			Interval:    service.HealthCheck.Interval,
			Timeout:     service.HealthCheck.Timeout,
			Retries:     service.HealthCheck.Retries,
			StartPeriod: service.HealthCheck.StartPeriod,
		}
	}

	// Create and start the container
	containerID, err := m.docker.CreateContainer(ctx, service.Name, containerConfig)
	if err != nil {
		return fmt.Errorf("failed to create container: %w", err)
	}

	if err := m.docker.StartContainer(ctx, containerID); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	// Record the deployment
	m.deployments[service.Name] = &Deployment{
		Name:          service.Name,
		ContainerID:   containerID,
		Image:         service.Image,
		CreatedAt:     time.Now(),
		ServiceType:   "service",
		DependsOn:     service.DependsOn,
		HealthCheck:   &service.HealthCheck,
		Environment:   service.Environment,
		Ports:         service.Ports,
		RestartPolicy: service.RestartPolicy,
	}

	// Perform health check if configured
	if service.HealthCheck.Type != "" {
		if err := m.performHealthCheck(ctx, service.Name, service.HealthCheck); err != nil {
			return fmt.Errorf("health check failed: %w", err)
		}
	}

	m.logger.Info("Service deployed successfully", "name", service.Name)
	return nil
}

// deployApp deploys a single application
func (m *Manager) deployApp(ctx context.Context, app config.App) error {
	m.logger.Info("Deploying application", "name", app.Name, "image", app.Image)

	// Pull the image
	if err := m.docker.PullImage(ctx, app.Image); err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}

	// Scan the image for security vulnerabilities
	scanResult, err := m.scanner.ScanImage(ctx, app.Image)
	if err != nil {
		m.logger.Warn("Failed to scan image", "name", app.Name, "error", err)
	} else if scanResult.CriticalCount > 0 {
		m.logger.Warn("Image has security vulnerabilities",
			"name", app.Name,
			"critical", scanResult.CriticalCount,
			"high", scanResult.HighCount)
	}

	// Create container config
	containerConfig := docker.ContainerConfig{
		Image:         app.Image,
		Environment:   app.Environment,
		Ports:         app.Ports,
		RestartPolicy: app.RestartPolicy,
		Labels: map[string]string{
			"shipyard.type": "app",
			"shipyard.name": app.Name,
		},
	}

	// Convert volumes if any
	for _, vol := range app.Volumes {
		containerConfig.Volumes = append(containerConfig.Volumes, docker.Volume{
			Source:      vol.Source,
			Destination: vol.Destination,
			Type:        vol.Type,
		})
	}

	// Create health check config if defined
	if app.HealthCheck.Type != "" {
		containerConfig.HealthCheck = &docker.HealthCheck{
			Type:        app.HealthCheck.Type,
			Path:        app.HealthCheck.Path,
			Port:        app.HealthCheck.Port,
			Interval:    app.HealthCheck.Interval,
			Timeout:     app.HealthCheck.Timeout,
			Retries:     app.HealthCheck.Retries,
			StartPeriod: app.HealthCheck.StartPeriod,
		}
	}

	// Create and start the container
	containerID, err := m.docker.CreateContainer(ctx, app.Name, containerConfig)
	if err != nil {
		return fmt.Errorf("failed to create container: %w", err)
	}

	if err := m.docker.StartContainer(ctx, containerID); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	// Record the deployment
	m.deployments[app.Name] = &Deployment{
		Name:          app.Name,
		ContainerID:   containerID,
		Image:         app.Image,
		CreatedAt:     time.Now(),
		ServiceType:   "app",
		DependsOn:     app.DependsOn,
		HealthCheck:   &app.HealthCheck,
		Environment:   app.Environment,
		Ports:         app.Ports,
		RestartPolicy: app.RestartPolicy,
	}

	// Perform health check if configured
	if app.HealthCheck.Type != "" {
		if err := m.performHealthCheck(ctx, app.Name, app.HealthCheck); err != nil {
			return fmt.Errorf("health check failed: %w", err)
		}
	}

	m.logger.Info("Application deployed successfully", "name", app.Name)
	return nil
}

// performHealthCheck executes a health check for a deployed container
func (m *Manager) performHealthCheck(ctx context.Context, name string, healthCfg config.HealthCheck) error {
	m.logger.Info("Performing health check", "name", name, "type", healthCfg.Type)

	options := health.CheckOptions{
		Type:        healthCfg.Type,
		Host:        name, // Use container name as host (Docker DNS)
		Port:        healthCfg.Port,
		Path:        healthCfg.Path,
		Timeout:     time.Duration(healthCfg.Timeout) * time.Second,
		Interval:    time.Duration(healthCfg.Interval) * time.Second,
		Retries:     healthCfg.Retries,
		StartPeriod: time.Duration(healthCfg.StartPeriod) * time.Second,
	}

	return m.health.Check(ctx, name, options)
}

// rollbackDeployment removes a single deployment
func (m *Manager) rollbackDeployment(ctx context.Context, deployment *Deployment) error {
	m.logger.Info("Rolling back deployment",
		"name", deployment.Name,
		"type", deployment.ServiceType)

	// Stop and remove the container
	if err := m.docker.StopContainer(ctx, deployment.ContainerID, 10); err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}

	if err := m.docker.RemoveContainer(ctx, deployment.ContainerID, true); err != nil {
		return fmt.Errorf("failed to remove container: %w", err)
	}

	// Remove the deployment record
	delete(m.deployments, deployment.Name)

	m.logger.Info("Rollback successful", "name", deployment.Name)
	return nil
}

// areDependenciesDeployed checks if all dependencies are deployed
func (m *Manager) areDependenciesDeployed(dependencies []string) bool {
	for _, dep := range dependencies {
		if _, exists := m.deployments[dep]; !exists {
			return false
		}
	}
	return true
}

// Close cleans up resources
func (m *Manager) Close() error {
	if m.docker != nil {
		return m.docker.Close()
	}
	return nil
}

// reverseServices returns services in reverse order
func reverseServices(services []config.Service) []config.Service {
	reversed := make([]config.Service, len(services))
	for i, j := 0, len(services)-1; i < len(services); i, j = i+1, j-1 {
		reversed[i] = services[j]
	}
	return reversed
}

// VerifyExternalAccess performs external verification checks for all deployed applications
func (m *Manager) VerifyExternalAccess(ctx context.Context) error {
	m.logger.Info("Starting external verification")

	// Create an external verifier to check public accessibility
	externalVerifier := health.NewExternalVerifier(m.config, m.logger)

	// Verify all applications are accessible from the internet
	if err := externalVerifier.VerifyExternalAccess(ctx); err != nil {
		m.logger.Error("External verification failed", "error", err)
		return err
	}

	m.logger.Info("External verification completed successfully")
	return nil
}
