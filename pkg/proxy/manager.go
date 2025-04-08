package proxy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/elijahmont3x/shipyard-action/pkg/config"
	"github.com/elijahmont3x/shipyard-action/pkg/docker"
	"github.com/elijahmont3x/shipyard-action/pkg/log"
)

// Manager handles the proxy setup and configuration
type Manager struct {
	logger      *log.Logger
	docker      *docker.Client
	config      *config.Config
	proxyType   string
	configPath  string
	containerID string
}

// NewManager creates a new proxy manager
func NewManager(logger *log.Logger, docker *docker.Client, cfg *config.Config) *Manager {
	return &Manager{
		logger:     logger.WithField("component", "proxy"),
		docker:     docker,
		config:     cfg,
		proxyType:  cfg.Proxy.Type,
		configPath: "/tmp/proxy-config",
	}
}

// Setup initializes the proxy
func (m *Manager) Setup(ctx context.Context) error {
	m.logger.Info("Setting up proxy server", "type", m.proxyType)

	// Create the config directory if it doesn't exist
	if err := createDirIfNotExists(m.configPath); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Generate the proxy configuration
	if err := m.generateConfig(); err != nil {
		return fmt.Errorf("failed to generate proxy configuration: %w", err)
	}

	// Start the proxy container
	containerID, err := m.startProxyContainer(ctx)
	if err != nil {
		return fmt.Errorf("failed to start proxy container: %w", err)
	}

	m.containerID = containerID
	return nil
}

// generateConfig creates the proxy configuration files
func (m *Manager) generateConfig() error {
	switch m.proxyType {
	case "nginx":
		return m.generateNginxConfig()
	case "traefik":
		return m.generateTraefikConfig()
	default:
		return fmt.Errorf("unsupported proxy type: %s", m.proxyType)
	}
}

// generateNginxConfig creates Nginx configuration
func (m *Manager) generateNginxConfig() error {
	m.logger.Info("Generating Nginx configuration")

	// Create nginx.conf
	nginxConfPath := filepath.Join(m.configPath, "nginx.conf")
	nginxConfFile, err := os.Create(nginxConfPath)
	if err != nil {
		return fmt.Errorf("failed to create nginx.conf: %w", err)
	}
	defer nginxConfFile.Close()

	// Define nginx.conf template
	nginxConfTmpl := `
user nginx;
worker_processes auto;
error_log /var/log/nginx/error.log notice;
pid /var/run/nginx.pid;

events {
    worker_connections 1024;
}

http {
    include /etc/nginx/mime.types;
    default_type application/octet-stream;
    log_format main '$remote_addr - $remote_user [$time_local] "$request" '
                    '$status $body_bytes_sent "$http_referer" '
                    '"$http_user_agent" "$http_x_forwarded_for"';
    access_log /var/log/nginx/access.log main;
    sendfile on;
    keepalive_timeout 65;
    
    # SSL settings
    ssl_session_cache shared:SSL:10m;
    ssl_session_timeout 10m;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_prefer_server_ciphers on;
    
    # Include virtual hosts
    include /etc/nginx/conf.d/*.conf;
}
`
	// Write the nginx.conf template
	if _, err := nginxConfFile.WriteString(nginxConfTmpl); err != nil {
		return fmt.Errorf("failed to write nginx.conf: %w", err)
	}

	// Create conf.d directory for virtual hosts
	confDir := filepath.Join(m.configPath, "conf.d")
	if err := createDirIfNotExists(confDir); err != nil {
		return fmt.Errorf("failed to create conf.d directory: %w", err)
	}

	// Create default.conf with server blocks for all apps
	defaultConfPath := filepath.Join(confDir, "default.conf")
	defaultConfFile, err := os.Create(defaultConfPath)
	if err != nil {
		return fmt.Errorf("failed to create default.conf: %w", err)
	}
	defer defaultConfFile.Close()

	// Create template for server blocks
	type ServerBlock struct {
		Domain    string
		Subdomain string
		Path      string
		Upstream  string
		Port      int
		UseSSL    bool
		SSLCert   string
		SSLKey    string
	}

	serverBlocks := []ServerBlock{}

	// Create server blocks for each app
	for _, app := range m.config.Apps {
		serverBlock := ServerBlock{
			Domain:    m.config.Domain,
			Subdomain: app.Subdomain,
			Path:      app.Path,
			Upstream:  app.Name,
			Port:      8080, // Default app port
			UseSSL:    m.config.SSL.Enabled,
		}

		// Extract port from the app's port mapping if available
		for _, portMapping := range app.Ports {
			parts := strings.Split(portMapping, ":")
			if len(parts) == 2 {
				fmt.Sscanf(parts[1], "%d", &serverBlock.Port)
				break
			}
		}

		// Set SSL certificate paths if SSL is enabled
		if serverBlock.UseSSL {
			serverBlock.SSLCert = fmt.Sprintf("/etc/ssl/certs/%s.crt", m.config.Domain)
			serverBlock.SSLKey = fmt.Sprintf("/etc/ssl/private/%s.key", m.config.Domain)
		}

		serverBlocks = append(serverBlocks, serverBlock)
	}

	// Create template for virtual host configuration
	vhostTmpl := `
{{ range . }}
# Server block for {{ if .Subdomain }}{{ .Subdomain }}.{{ end }}{{ .Domain }}{{ if .Path }}{{ .Path }}{{ end }}
server {
    {{ if .UseSSL }}
    listen 443 ssl;
    ssl_certificate {{ .SSLCert }};
    ssl_certificate_key {{ .SSLKey }};
    {{ else }}
    listen 80;
    {{ end }}
    
    {{ if .Subdomain }}
    server_name {{ .Subdomain }}.{{ .Domain }};
    {{ else }}
    server_name {{ .Domain }};
    {{ end }}
    
    {{ if .Path }}
    location {{ .Path }} {
        proxy_pass http://{{ .Upstream }}:{{ .Port }};
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
    {{ else }}
    location / {
        proxy_pass http://{{ .Upstream }}:{{ .Port }};
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
    {{ end }}
}

{{ if .UseSSL }}
# Redirect HTTP to HTTPS
server {
    listen 80;
    {{ if .Subdomain }}
    server_name {{ .Subdomain }}.{{ .Domain }};
    {{ else }}
    server_name {{ .Domain }};
    {{ end }}
    return 301 https://$host$request_uri;
}
{{ end }}
{{ end }}
`

	// Parse and execute the template
	tmpl, err := template.New("vhost").Parse(vhostTmpl)
	if err != nil {
		return fmt.Errorf("failed to parse virtual host template: %w", err)
	}

	if err := tmpl.Execute(defaultConfFile, serverBlocks); err != nil {
		return fmt.Errorf("failed to generate virtual host configuration: %w", err)
	}

	return nil
}

// generateTraefikConfig creates Traefik configuration
func (m *Manager) generateTraefikConfig() error {
	m.logger.Info("Generating Traefik configuration")

	// Create traefik.toml
	traefikConfPath := filepath.Join(m.configPath, "traefik.toml")
	traefikConfFile, err := os.Create(traefikConfPath)
	if err != nil {
		return fmt.Errorf("failed to create traefik.toml: %w", err)
	}
	defer traefikConfFile.Close()

	// Define Traefik template
	traefikConfTmpl := `
[global]
  checkNewVersion = false
  sendAnonymousUsage = false

[entryPoints]
  [entryPoints.web]
    address = ":80"
    {{ if .SSL.Enabled }}
    [entryPoints.web.http.redirections.entryPoint]
      to = "websecure"
      scheme = "https"
    {{ end }}
  
  {{ if .SSL.Enabled }}
  [entryPoints.websecure]
    address = ":443"
    [entryPoints.websecure.http.tls]
  {{ end }}

[api]
  dashboard = false

[providers]
  [providers.file]
    filename = "/etc/traefik/routes.toml"

{{ if .SSL.Enabled }}
[certificatesResolvers.shipyard.acme]
  email = "{{ .SSL.Email }}"
  storage = "/etc/traefik/acme.json"
  {{ if .SSL.DNSChallenge }}
  [certificatesResolvers.shipyard.acme.dnsChallenge]
    provider = "{{ .SSL.DNSProvider }}"
    delayBeforeCheck = 30
  {{ else }}
  [certificatesResolvers.shipyard.acme.httpChallenge]
    entryPoint = "web"
  {{ end }}
{{ end }}

[log]
  level = "INFO"
`

	// Execute the Traefik template
	tmpl, err := template.New("traefik").Parse(traefikConfTmpl)
	if err != nil {
		return fmt.Errorf("failed to parse Traefik template: %w", err)
	}

	if err := tmpl.Execute(traefikConfFile, m.config); err != nil {
		return fmt.Errorf("failed to generate Traefik configuration: %w", err)
	}

	// Create routes.toml for service definitions
	routesConfPath := filepath.Join(m.configPath, "routes.toml")
	routesConfFile, err := os.Create(routesConfPath)
	if err != nil {
		return fmt.Errorf("failed to create routes.toml: %w", err)
	}
	defer routesConfFile.Close()

	// Generate routes configuration
	routesConf := `
[http]
  [http.services]
`

	// Add services for each app
	for _, app := range m.config.Apps {
		port := 8080 // Default port

		// Extract port from the app's port mapping if available
		for _, portMapping := range app.Ports {
			parts := strings.Split(portMapping, ":")
			if len(parts) == 2 {
				fmt.Sscanf(parts[1], "%d", &port)
				break
			}
		}

		routesConf += fmt.Sprintf(`
    [http.services.%s.loadBalancer]
      [[http.services.%s.loadBalancer.servers]]
        url = "http://%s:%d"
`, app.Name, app.Name, app.Name, port)
	}

	routesConf += `
  [http.routers]
`

	// Add routers for each app
	for _, app := range m.config.Apps {
		rule := ""
		if app.Subdomain != "" {
			rule = fmt.Sprintf(`Host("%s.%s")`, app.Subdomain, m.config.Domain)
		} else if app.Path != "" {
			rule = fmt.Sprintf(`Host("%s") && PathPrefix("%s")`, m.config.Domain, app.Path)
		} else {
			rule = fmt.Sprintf(`Host("%s")`, m.config.Domain)
		}

		entrypoint := "web"
		tls := ""

		if m.config.SSL.Enabled {
			entrypoint = "websecure"
			tls = `
      [http.routers.` + app.Name + `.tls]
        certResolver = "shipyard"
`
		}

		routesConf += fmt.Sprintf(`
    [http.routers.%s]
      rule = "%s"
      service = "%s"
      entryPoints = ["%s"]%s
`, app.Name, rule, app.Name, entrypoint, tls)
	}

	// Write the routes configuration
	if _, err := routesConfFile.WriteString(routesConf); err != nil {
		return fmt.Errorf("failed to write routes.toml: %w", err)
	}

	return nil
}

// startProxyContainer starts the proxy container
func (m *Manager) startProxyContainer(ctx context.Context) (string, error) {
	m.logger.Info("Starting proxy container", "type", m.proxyType)

	// Define container config based on proxy type
	var image string
	var ports []string
	var volumes []docker.Volume
	var env map[string]string

	switch m.proxyType {
	case "nginx":
		image = "nginx:alpine"
		ports = []string{
			fmt.Sprintf("%d:80", m.config.Proxy.Port),
		}

		volumes = []docker.Volume{
			{
				Source:      filepath.Join(m.configPath, "nginx.conf"),
				Destination: "/etc/nginx/nginx.conf",
				Type:        "bind",
			},
			{
				Source:      filepath.Join(m.configPath, "conf.d"),
				Destination: "/etc/nginx/conf.d",
				Type:        "bind",
			},
		}

		if m.config.SSL.Enabled {
			ports = append(ports, fmt.Sprintf("%d:443", m.config.Proxy.HTTPSPort))
			volumes = append(volumes, docker.Volume{
				Source:      "/etc/shipyard/ssl/certs",
				Destination: "/etc/ssl/certs",
				Type:        "bind",
			}, docker.Volume{
				Source:      "/etc/shipyard/ssl/private",
				Destination: "/etc/ssl/private",
				Type:        "bind",
			})
		}

	case "traefik":
		image = "traefik:v2.9"
		ports = []string{
			fmt.Sprintf("%d:80", m.config.Proxy.Port),
		}

		volumes = []docker.Volume{
			{
				Source:      filepath.Join(m.configPath, "traefik.toml"),
				Destination: "/etc/traefik/traefik.toml",
				Type:        "bind",
			},
			{
				Source:      filepath.Join(m.configPath, "routes.toml"),
				Destination: "/etc/traefik/routes.toml",
				Type:        "bind",
			},
			{
				Source:      "/var/run/docker.sock",
				Destination: "/var/run/docker.sock",
				Type:        "bind",
			},
		}

		env = map[string]string{}

		if m.config.SSL.Enabled {
			ports = append(ports, fmt.Sprintf("%d:443", m.config.Proxy.HTTPSPort))

			// Add environment variables for DNS provider if using DNS challenge
			if m.config.SSL.DNSChallenge {
				for k, v := range m.config.SSL.DNSCredentials {
					env[k] = v
				}
			}
		}

	default:
		return "", fmt.Errorf("unsupported proxy type: %s", m.proxyType)
	}

	// Create the proxy container
	containerConfig := docker.ContainerConfig{
		Image:       image,
		Environment: env,
		Ports:       ports,
		Volumes:     volumes,
		Labels: map[string]string{
			"shipyard.type": "proxy",
		},
		RestartPolicy: "unless-stopped",
	}

	containerID, err := m.docker.CreateContainer(ctx, "shipyard-proxy", containerConfig)
	if err != nil {
		return "", fmt.Errorf("failed to create proxy container: %w", err)
	}

	// Start the container
	if err := m.docker.StartContainer(ctx, containerID); err != nil {
		return "", fmt.Errorf("failed to start proxy container: %w", err)
	}

	return containerID, nil
}

// Cleanup removes the proxy container and configuration
func (m *Manager) Cleanup(ctx context.Context) error {
	if m.containerID != "" {
		m.logger.Info("Cleaning up proxy resources", "container", m.containerID)

		// Stop and remove the proxy container
		if err := m.docker.StopContainer(ctx, m.containerID, 10); err != nil {
			m.logger.Error("Failed to stop proxy container", "error", err)
		}

		if err := m.docker.RemoveContainer(ctx, m.containerID, true); err != nil {
			m.logger.Error("Failed to remove proxy container", "error", err)
		}
	}

	// Remove the configuration directory
	if err := os.RemoveAll(m.configPath); err != nil {
		m.logger.Error("Failed to remove proxy configuration", "error", err)
	}

	return nil
}

// Reload reloads the proxy configuration
func (m *Manager) Reload(ctx context.Context) error {
	m.logger.Info("Reloading proxy configuration")

	// Regenerate the proxy configuration
	if err := m.generateConfig(); err != nil {
		return fmt.Errorf("failed to regenerate proxy configuration: %w", err)
	}

	// Execute reload command based on proxy type
	switch m.proxyType {
	case "nginx":
		m.logger.Info("Reloading Nginx configuration")
		exitCode, stdout, stderr, err := m.docker.Execute(ctx, m.containerID, []string{"nginx", "-s", "reload"})

		if err != nil {
			return fmt.Errorf("failed to execute Nginx reload: %w", err)
		}

		if exitCode != 0 {
			m.logger.Error("Nginx reload failed", "exitCode", exitCode, "stderr", stderr)
			return fmt.Errorf("nginx reload failed with exit code %d: %s", exitCode, stderr)
		}

		m.logger.Debug("Nginx reload succeeded", "stdout", stdout)
		return nil

	case "traefik":
		// Traefik automatically reloads when configuration changes
		m.logger.Info("Traefik config change detected - automatic reload will happen")
		return nil

	default:
		return fmt.Errorf("unsupported proxy type: %s", m.proxyType)
	}
}

// createDirIfNotExists creates a directory if it doesn't exist
func createDirIfNotExists(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return os.MkdirAll(path, 0755)
	}
	return nil
}
