package config

import (
	"fmt"
	"strings"
)

// Validate performs validation on the configuration
func Validate(config *Config) error {
	if config.Version == "" {
		return fmt.Errorf("config version is required")
	}

	if config.Domain == "" {
		return fmt.Errorf("domain is required")
	}

	// Validate services
	serviceNames := make(map[string]bool)
	for i, service := range config.Services {
		if service.Name == "" {
			return fmt.Errorf("service at index %d is missing a name", i)
		}

		if serviceNames[service.Name] {
			return fmt.Errorf("duplicate service name: %s", service.Name)
		}
		serviceNames[service.Name] = true

		if service.Image == "" {
			return fmt.Errorf("service %s is missing an image", service.Name)
		}

		// Validate service dependencies
		for _, dep := range service.DependsOn {
			if !serviceNames[dep] {
				return fmt.Errorf("service %s depends on undefined service: %s", service.Name, dep)
			}
		}
	}

	// Validate apps
	appNames := make(map[string]bool)
	subdomains := make(map[string]bool)
	paths := make(map[string]bool)

	for i, app := range config.Apps {
		if app.Name == "" {
			return fmt.Errorf("app at index %d is missing a name", i)
		}

		if appNames[app.Name] {
			return fmt.Errorf("duplicate app name: %s", app.Name)
		}
		appNames[app.Name] = true

		if app.Image == "" {
			return fmt.Errorf("app %s is missing an image", app.Name)
		}

		// Check for routing conflicts
		if app.Subdomain != "" {
			if subdomains[app.Subdomain] {
				return fmt.Errorf("duplicate subdomain: %s", app.Subdomain)
			}
			subdomains[app.Subdomain] = true
		}

		if app.Path != "" {
			normalizedPath := normalizePath(app.Path)
			if paths[normalizedPath] {
				return fmt.Errorf("duplicate path: %s", app.Path)
			}
			paths[normalizedPath] = true
		}

		// Validate app dependencies
		for _, dep := range app.DependsOn {
			if !serviceNames[dep] && !appNames[dep] {
				return fmt.Errorf("app %s depends on undefined service or app: %s", app.Name, dep)
			}
		}
	}

	// Validate SSL configuration
	if config.SSL.Enabled {
		if !config.SSL.SelfSigned && config.SSL.Email == "" {
			return fmt.Errorf("SSL email is required when using Let's Encrypt")
		}

		if config.SSL.DNSChallenge && config.SSL.DNSProvider == "" {
			return fmt.Errorf("DNS provider is required when using DNS challenge")
		}
	}

	return nil
}

// normalizePath standardizes a path for comparison
func normalizePath(path string) string {
	// Ensure path starts with /
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	// Remove trailing slash except for root path
	if path != "/" && strings.HasSuffix(path, "/") {
		path = path[:len(path)-1]
	}

	return path
}
