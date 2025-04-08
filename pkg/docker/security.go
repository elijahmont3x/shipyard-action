package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/elijahmont3x/shipyard-action/pkg/log"
)

// SecurityScanner provides security scanning for Docker images
type SecurityScanner struct {
	logger *log.Logger
}

// NewSecurityScanner creates a new security scanner
func NewSecurityScanner(logger *log.Logger) *SecurityScanner {
	return &SecurityScanner{
		logger: logger.WithField("component", "security-scanner"),
	}
}

// ScanImage scans a Docker image for security vulnerabilities using Trivy
type ScanResult struct {
	Vulnerabilities int
	CriticalCount   int
	HighCount       int
	MediumCount     int
	LowCount        int
	Details         string
}

// ScanImage scans an image for vulnerabilities
func (s *SecurityScanner) ScanImage(ctx context.Context, image string) (*ScanResult, error) {
	// Check if security scanning is disabled
	if os.Getenv("INPUT_SKIP_SECURITY_SCAN") == "true" {
		s.logger.Info("Security scanning is disabled, skipping", "image", image)
		return &ScanResult{}, nil
	}

	s.logger.Info("Scanning image for security vulnerabilities", "image", image)

	// Check if Trivy is installed
	if _, err := exec.LookPath("trivy"); err != nil {
		// Install Trivy if not already installed
		err = s.installTrivy(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to install Trivy: %w", err)
		}
	}

	// Run Trivy scan
	cmd := exec.CommandContext(
		ctx,
		"trivy",
		"image",
		"--format", "json",
		"--quiet",
		image,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("trivy scan failed: %w\noutput: %s", err, string(output))
	}

	// Parse the results
	var result struct {
		Results []struct {
			Vulnerabilities []struct {
				Severity string `json:"Severity"`
			} `json:"Vulnerabilities"`
		} `json:"Results"`
	}

	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("failed to parse Trivy output: %w", err)
	}

	// Count vulnerabilities by severity
	var criticalCount, highCount, mediumCount, lowCount int
	var totalCount int

	for _, r := range result.Results {
		for _, v := range r.Vulnerabilities {
			totalCount++
			switch strings.ToLower(v.Severity) {
			case "critical":
				criticalCount++
			case "high":
				highCount++
			case "medium":
				mediumCount++
			case "low":
				lowCount++
			}
		}
	}

	scanResult := &ScanResult{
		Vulnerabilities: totalCount,
		CriticalCount:   criticalCount,
		HighCount:       highCount,
		MediumCount:     mediumCount,
		LowCount:        lowCount,
		Details:         string(output),
	}

	s.logger.Info("Scan completed",
		"image", image,
		"vulnerabilities", totalCount,
		"critical", criticalCount,
		"high", highCount)

	return scanResult, nil
}

// installTrivy installs the Trivy scanner
func (s *SecurityScanner) installTrivy(ctx context.Context) error {
	s.logger.Info("Installing Trivy scanner")

	// Check the operating system
	cmd := exec.CommandContext(
		ctx,
		"sh",
		"-c",
		"curl -sfL https://raw.githubusercontent.com/aquasecurity/trivy/main/contrib/install.sh | sh -s -- -b /usr/local/bin",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to install Trivy: %w\nOutput: %s", err, string(output))
	}

	s.logger.Info("Trivy installation completed")
	return nil
}
