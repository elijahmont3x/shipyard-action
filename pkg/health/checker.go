package health

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/elijahmont3x/shipyard-action/pkg/log"
)

// Checker manages health checks for services and applications
type Checker struct {
	logger *log.Logger
}

// NewChecker creates a new health checker
func NewChecker(logger *log.Logger) *Checker {
	return &Checker{
		logger: logger.WithField("component", "health"),
	}
}

// CheckOptions defines options for health checking
type CheckOptions struct {
	Type        string
	Host        string
	Port        int
	Path        string
	Timeout     time.Duration
	Interval    time.Duration
	Retries     int
	StartPeriod time.Duration
}

// Check performs health checks on a service
func (c *Checker) Check(ctx context.Context, name string, options CheckOptions) error {
	c.logger.Info("Starting health check", "name", name, "type", options.Type)

	// Wait for start period before beginning health checks
	if options.StartPeriod > 0 {
		c.logger.Debug("Waiting for start period", "name", name, "duration", options.StartPeriod)
		select {
		case <-time.After(options.StartPeriod):
			// Continue with health check
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// Perform health checks with retry
	var lastErr error
	for attempt := 0; attempt < options.Retries; attempt++ {
		if attempt > 0 {
			c.logger.Debug("Retrying health check",
				"name", name,
				"attempt", attempt+1,
				"max", options.Retries)

			// Wait before retrying
			select {
			case <-time.After(options.Interval):
				// Continue with next retry
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		// Perform the appropriate health check
		var err error
		switch options.Type {
		case "http":
			err = c.checkHTTP(ctx, options)
		case "tcp":
			err = c.checkTCP(ctx, options)
		default:
			return fmt.Errorf("unsupported health check type: %s", options.Type)
		}

		// If health check succeeds, we're done
		if err == nil {
			c.logger.Info("Health check passed", "name", name, "attempt", attempt+1)
			return nil
		}

		lastErr = err
		c.logger.Debug("Health check failed",
			"name", name,
			"attempt", attempt+1,
			"error", err.Error())
	}

	c.logger.Error("Health check failed after retries",
		"name", name,
		"retries", options.Retries)

	return fmt.Errorf("health check failed: %w", lastErr)
}

// checkHTTP performs an HTTP health check
func (c *Checker) checkHTTP(ctx context.Context, options CheckOptions) error {
	url := fmt.Sprintf("http://%s:%d%s", options.Host, options.Port, options.Path)

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: options.Timeout,
	}

	// Create request with context for cancellation
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Perform HTTP request
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP health check failed with status: %d", resp.StatusCode)
	}

	return nil
}

// checkTCP performs a TCP health check
func (c *Checker) checkTCP(ctx context.Context, options CheckOptions) error {
	// Create a deadline context for the connection
	dialCtx, cancel := context.WithTimeout(ctx, options.Timeout)
	defer cancel()

	// Try to establish a TCP connection
	address := fmt.Sprintf("%s:%d", options.Host, options.Port)
	var d net.Dialer
	conn, err := d.DialContext(dialCtx, "tcp", address)
	if err != nil {
		return fmt.Errorf("TCP connection failed: %w", err)
	}
	defer conn.Close()

	return nil
}
