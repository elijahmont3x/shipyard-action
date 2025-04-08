package end

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	"github.com/elijahmont3x/shipyard-action/pkg/config"
	"github.com/elijahmont3x/shipyard-action/pkg/log"
)

// ExternalVerifier performs external HTTP checks against deployed applications
type ExternalVerifier struct {
	config *config.Config
	logger *log.Logger // Changed to match other components
	client *http.Client
}

// NewExternalVerifier creates a new external verifier
func NewExternalVerifier(cfg *config.Config, logger *log.Logger) *ExternalVerifier {
	// Create HTTP client with reasonable timeout and TLS settings
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				// Skip cert verification for non-production environments
				// or when using self-signed certificates
				InsecureSkipVerify: cfg.SSL.SelfSigned,
			},
		},
	}

	return &ExternalVerifier{
		config: cfg,
		logger: logger.WithField("component", "external-verifier"),
		client: client,
	}
}

// VerifyExternalAccess checks that all deployed applications are accessible from the internet
func (v *ExternalVerifier) VerifyExternalAccess(ctx context.Context) error {
	v.logger.Info("Starting external verification checks")

	// Default protocol based on SSL settings
	protocol := "http"
	if v.config.SSL.Enabled {
		protocol = "https"
	}

	// Verify each app
	for _, app := range v.config.Apps {
		// Build the URL based on app configuration
		var url string
		if app.Subdomain == "" {
			url = fmt.Sprintf("%s://%s", protocol, v.config.Domain)
		} else {
			url = fmt.Sprintf("%s://%s.%s", protocol, app.Subdomain, v.config.Domain)
		}

		// Add path if specified
		if app.Path != "" && app.Path != "/" {
			url = fmt.Sprintf("%s%s", url, app.Path)
		}

		// Add health check path for verification if it exists
		healthPath := "/"
		if app.HealthCheck.Type == "http" && app.HealthCheck.Path != "" {
			healthPath = app.HealthCheck.Path
		}

		// Check main URL
		mainUrl := url
		if err := v.checkEndpoint(ctx, mainUrl, 5, 3); err != nil {
			return fmt.Errorf("external verification failed for app %s at %s: %w", app.Name, mainUrl, err)
		}

		// Check health URL if different from main URL
		if healthPath != "/" && healthPath != "" {
			healthUrl := fmt.Sprintf("%s%s", url, healthPath)
			if err := v.checkEndpoint(ctx, healthUrl, 5, 3); err != nil {
				return fmt.Errorf("health check verification failed for app %s at %s: %w", app.Name, healthUrl, err)
			}
		}
	}

	v.logger.Info("External verification completed successfully")
	return nil
}

// checkEndpoint verifies that a URL is accessible with retries and backoff
func (v *ExternalVerifier) checkEndpoint(ctx context.Context, url string, maxAttempts int, initialWaitSeconds int) error {
	waitTime := time.Duration(initialWaitSeconds) * time.Second

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		v.logger.Info(fmt.Sprintf("Verifying external endpoint (attempt %d/%d): %s", attempt, maxAttempts, url))

		// Check if context is cancelled
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Continue with verification
		}

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			v.logger.Error(fmt.Sprintf("Failed to create request: %v", err))
			return err
		}

		resp, err := v.client.Do(req)
		if err != nil {
			v.logger.Warn(fmt.Sprintf("Endpoint check failed: %v. Retrying in %v...", err, waitTime))
			time.Sleep(waitTime)
			// Exponential backoff
			waitTime = waitTime + (waitTime / 2)
			continue
		}
		defer resp.Body.Close()

		// Check for successful status code (2xx)
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			v.logger.Info(fmt.Sprintf("✅ Successfully verified %s (Status: %d)", url, resp.StatusCode))
			return nil
		}

		v.logger.Warn(fmt.Sprintf("Endpoint returned non-success status %d. Retrying in %v...", resp.StatusCode, waitTime))
		time.Sleep(waitTime)
		// Exponential backoff
		waitTime = waitTime + (waitTime / 2)
	}

	return fmt.Errorf("endpoint verification failed after %d attempts", maxAttempts)
}
