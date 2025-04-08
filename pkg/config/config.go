package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the top-level configuration for Shipyard
type Config struct {
	Version  string      `yaml:"version"`
	Domain   string      `yaml:"domain"`
	SSL      SSLConfig   `yaml:"ssl"`
	Proxy    ProxyConfig `yaml:"proxy"`
	Services []Service   `yaml:"services"`
	Apps     []App       `yaml:"apps"`
	Env      EnvConfig   `yaml:"env"`
	Timeout  int         `yaml:"timeout"`
	LogLevel string      `yaml:"logLevel"`
}

// SSLConfig represents SSL certificate configuration
type SSLConfig struct {
	Enabled        bool              `yaml:"enabled"`
	Provider       string            `yaml:"provider"`
	Email          string            `yaml:"email"`
	SelfSigned     bool              `yaml:"selfSigned"`
	DNSChallenge   bool              `yaml:"dnsChallenge"`
	DNSProvider    string            `yaml:"dnsProvider"`
	DNSCredentials map[string]string `yaml:"dnsCredentials"`
}

// ProxyConfig represents the proxy configuration
type ProxyConfig struct {
	Type      string `yaml:"type"`
	Port      int    `yaml:"port"`
	HTTPSPort int    `yaml:"httpsPort"`
}

// Service represents a persistent service like a database
type Service struct {
	Name          string            `yaml:"name"`
	Image         string            `yaml:"image"`
	Volumes       []Volume          `yaml:"volumes"`
	Environment   map[string]string `yaml:"environment"`
	Ports         []string          `yaml:"ports"`
	DependsOn     []string          `yaml:"dependsOn"`
	HealthCheck   HealthCheck       `yaml:"healthCheck"`
	RestartPolicy string            `yaml:"restartPolicy"`
}

// App represents a deployed application
type App struct {
	Name          string            `yaml:"name"`
	Image         string            `yaml:"image"`
	Subdomain     string            `yaml:"subdomain"`
	Path          string            `yaml:"path"`
	Environment   map[string]string `yaml:"environment"`
	Ports         []string          `yaml:"ports"`
	Volumes       []Volume          `yaml:"volumes"`
	DependsOn     []string          `yaml:"dependsOn"`
	HealthCheck   HealthCheck       `yaml:"healthCheck"`
	RestartPolicy string            `yaml:"restartPolicy"`
}

// Volume represents a Docker volume configuration
type Volume struct {
	Source      string `yaml:"source"`
	Destination string `yaml:"destination"`
	Type        string `yaml:"type"`
}

// HealthCheck represents health check configuration
type HealthCheck struct {
	Type        string `yaml:"type"`
	Path        string `yaml:"path"`
	Port        int    `yaml:"port"`
	Interval    int    `yaml:"interval"`
	Timeout     int    `yaml:"timeout"`
	Retries     int    `yaml:"retries"`
	StartPeriod int    `yaml:"startPeriod"`
}

// EnvConfig represents environment variables configuration
type EnvConfig struct {
	Global map[string]string `yaml:"global"`
}

// Load loads the configuration from a file and GitHub Actions inputs
func Load() (*Config, error) {
	configPath := os.Getenv("INPUT_CONFIG")
	if configPath == "" {
		configPath = ".shipyard/config.yml"
	}

	// Load from file
	config, err := loadFromFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config from file: %w", err)
	}

	// Apply GitHub Action inputs
	applyInputs(config)

	// Validate the configuration
	if err := Validate(config); err != nil {
		return nil, err
	}

	// Apply defaults for missing values
	ApplyDefaults(config)

	return config, nil
}

// loadFromFile loads the configuration from the specified YAML file
func loadFromFile(path string) (*Config, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &config, nil
}

// applyInputs overrides configuration with GitHub Action inputs
func applyInputs(config *Config) {
	if logLevel := os.Getenv("INPUT_LOG_LEVEL"); logLevel != "" {
		config.LogLevel = logLevel
	}

	if timeout := os.Getenv("INPUT_TIMEOUT"); timeout != "" {
		// Parse timeout and set it (error handling omitted for brevity)
		var timeoutVal int
		fmt.Sscanf(timeout, "%d", &timeoutVal)
		if timeoutVal > 0 {
			config.Timeout = timeoutVal
		}
	}

	if dnsProvider := os.Getenv("INPUT_DNS_PROVIDER"); dnsProvider != "" && dnsProvider != "none" {
		config.SSL.DNSProvider = dnsProvider
		config.SSL.DNSChallenge = true
	}
}
