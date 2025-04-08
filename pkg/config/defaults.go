package config

import "strings"

// ApplyDefaults sets default values for missing configuration
func ApplyDefaults(config *Config) {
	// Default version if not specified
	if config.Version == "" {
		config.Version = "1.0"
	}

	// Default timeout
	if config.Timeout <= 0 {
		config.Timeout = 30 // 30 minutes
	}

	// Default log level
	if config.LogLevel == "" {
		config.LogLevel = "info"
	}

	// Default proxy settings
	if config.Proxy.Type == "" {
		config.Proxy.Type = "nginx"
	}
	if config.Proxy.Port <= 0 {
		config.Proxy.Port = 80
	}
	if config.Proxy.HTTPSPort <= 0 {
		config.Proxy.HTTPSPort = 443
	}

	// Apply default SSL settings
	if config.SSL.Enabled {
		// Default to HTTP challenge if not specified
		if !config.SSL.DNSChallenge && config.SSL.DNSProvider == "" {
			config.SSL.DNSChallenge = false
		}
	}

	// Apply service defaults
	for i := range config.Services {
		applyServiceDefaults(&config.Services[i])
	}

	// Apply app defaults
	for i := range config.Apps {
		applyAppDefaults(&config.Apps[i])
	}
}

// applyServiceDefaults sets defaults for a service
func applyServiceDefaults(service *Service) {
	// Default restart policy
	if service.RestartPolicy == "" {
		service.RestartPolicy = "unless-stopped"
	}

	// Apply health check defaults based on service type
	applyHealthCheckDefaults(service.Name, service.Image, &service.HealthCheck)
}

// applyAppDefaults sets defaults for an app
func applyAppDefaults(app *App) {
	// Default restart policy
	if app.RestartPolicy == "" {
		app.RestartPolicy = "unless-stopped"
	}

	// If no routing is specified, use the app name as subdomain
	if app.Subdomain == "" && app.Path == "" {
		app.Subdomain = app.Name
	}

	// Apply health check defaults
	applyHealthCheckDefaults(app.Name, app.Image, &app.HealthCheck)
}

// applyHealthCheckDefaults sets sensible health check defaults based on service type
func applyHealthCheckDefaults(name, image string, health *HealthCheck) {
	// Skip if already configured
	if health.Type != "" {
		return
	}

	// Default values
	health.Interval = 30
	health.Timeout = 10
	health.Retries = 3
	health.StartPeriod = 60

	// Set type-specific defaults based on the image name
	switch {
	case contains(image, "postgres", "postgresql"):
		health.Type = "tcp"
		health.Port = 5432
	case contains(image, "mysql", "mariadb"):
		health.Type = "tcp"
		health.Port = 3306
	case contains(image, "mongo", "mongodb"):
		health.Type = "tcp"
		health.Port = 27017
	case contains(image, "redis"):
		health.Type = "tcp"
		health.Port = 6379
	case contains(image, "rabbitmq"):
		health.Type = "tcp"
		health.Port = 5672
	case contains(image, "nginx", "traefik"):
		health.Type = "http"
		health.Path = "/health"
		health.Port = 80
	default:
		// Default to HTTP for apps, TCP for services
		if strings.HasPrefix(name, "app-") {
			health.Type = "http"
			health.Path = "/health"
			health.Port = 8080
		} else {
			health.Type = "tcp"
			health.Port = 8080
		}
	}
}

// contains checks if a string contains any of the substrings
func contains(s string, substrings ...string) bool {
	s = strings.ToLower(s)
	for _, sub := range substrings {
		if strings.Contains(s, strings.ToLower(sub)) {
			return true
		}
	}
	return false
}
