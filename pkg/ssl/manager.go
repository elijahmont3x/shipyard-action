package ssl

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/elijahmont3x/shipyard-action/pkg/config"
	"github.com/elijahmont3x/shipyard-action/pkg/log"
)

// Manager handles SSL certificate operations
type Manager struct {
	logger  *log.Logger
	config  *config.SSLConfig
	domain  string
	certDir string
	keyDir  string
}

// NewManager creates a new SSL manager
func NewManager(logger *log.Logger, cfg *config.Config) *Manager {
	certDir := "/etc/shipyard/ssl/certs"
	keyDir := "/etc/shipyard/ssl/private"

	return &Manager{
		logger:  logger.WithField("component", "ssl"),
		config:  &cfg.SSL,
		domain:  cfg.Domain,
		certDir: certDir,
		keyDir:  keyDir,
	}
}

// Setup initializes SSL certificate directories and obtains certificates
func (m *Manager) Setup(ctx context.Context) error {
	if !m.config.Enabled {
		m.logger.Info("SSL is not enabled, skipping setup")
		return nil
	}

	m.logger.Info("Setting up SSL certificates", "domain", m.domain)

	// Create certificate directories
	if err := ensureDir(m.certDir); err != nil {
		return fmt.Errorf("failed to create cert directory: %w", err)
	}

	if err := ensureDir(m.keyDir); err != nil {
		return fmt.Errorf("failed to create key directory: %w", err)
	}

	// Check if we already have valid certificates
	if m.hasValidCertificates() {
		m.logger.Info("Valid certificates already exist")
		return nil
	}

	// Obtain certificates
	if m.config.SelfSigned {
		return m.createSelfSignedCert()
	}

	return m.obtainLetsEncryptCert(ctx)
}

// hasValidCertificates checks if valid certificates exist
func (m *Manager) hasValidCertificates() bool {
	certPath := filepath.Join(m.certDir, fmt.Sprintf("%s.crt", m.domain))
	keyPath := filepath.Join(m.keyDir, fmt.Sprintf("%s.key", m.domain))

	// Check if both files exist
	certInfo, certErr := os.Stat(certPath)
	keyInfo, keyErr := os.Stat(keyPath)
	if certErr != nil || keyErr != nil {
		return false
	}

	// Check if cert file is empty
	if certInfo.Size() == 0 || keyInfo.Size() == 0 {
		return false
	}

	// TODO: Check certificate expiration

	return true
}

// createSelfSignedCert generates a self-signed certificate
func (m *Manager) createSelfSignedCert() error {
	m.logger.Info("Creating self-signed certificate", "domain", m.domain)

	// Generate private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("failed to generate private key: %w", err)
	}

	// Create certificate template
	notBefore := time.Now()
	notAfter := notBefore.Add(365 * 24 * time.Hour) // Valid for 1 year

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return fmt.Errorf("failed to generate serial number: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Shipyard Deployment"},
			CommonName:   m.domain,
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{m.domain, "*." + m.domain},
	}

	// Create certificate
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return fmt.Errorf("failed to create certificate: %w", err)
	}

	// Write certificate to file
	certPath := filepath.Join(m.certDir, fmt.Sprintf("%s.crt", m.domain))
	certOut, err := os.Create(certPath)
	if err != nil {
		return fmt.Errorf("failed to open cert file for writing: %w", err)
	}
	defer certOut.Close()

	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		return fmt.Errorf("failed to write cert file: %w", err)
	}

	// Write private key to file
	keyPath := filepath.Join(m.keyDir, fmt.Sprintf("%s.key", m.domain))
	keyOut, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to open key file for writing: %w", err)
	}
	defer keyOut.Close()

	privBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}

	if err := pem.Encode(keyOut, privBlock); err != nil {
		return fmt.Errorf("failed to write key file: %w", err)
	}

	m.logger.Info("Self-signed certificate created successfully", "domain", m.domain)
	return nil
}

// obtainLetsEncryptCert obtains a certificate from Let's Encrypt
func (m *Manager) obtainLetsEncryptCert(ctx context.Context) error {
	m.logger.Info("Obtaining Let's Encrypt certificate",
		"domain", m.domain,
		"email", m.config.Email,
		"dnsChallenge", m.config.DNSChallenge)

	// Create a new ACME client
	client, err := newAcmeClient(m.config.Email)
	if err != nil {
		return fmt.Errorf("failed to create ACME client: %w", err)
	}

	// Setup the challenge type
	var solver Solver
	if m.config.DNSChallenge {
		solver, err = newDNSSolver(m.config.DNSProvider, m.config.DNSCredentials)
	} else {
		solver, err = newHTTPSolver()
	}

	if err != nil {
		return fmt.Errorf("failed to create solver: %w", err)
	}

	// Set the solver on the client
	client.SetSolver(solver)

	// Obtain the certificate
	cert, err := client.ObtainCertificate(ctx, m.domain)
	if err != nil {
		return fmt.Errorf("failed to obtain certificate: %w", err)
	}

	// Write certificate to file
	certPath := filepath.Join(m.certDir, fmt.Sprintf("%s.crt", m.domain))
	if err := os.WriteFile(certPath, cert.Certificate, 0644); err != nil {
		return fmt.Errorf("failed to write certificate file: %w", err)
	}

	// Write private key to file
	keyPath := filepath.Join(m.keyDir, fmt.Sprintf("%s.key", m.domain))
	if err := os.WriteFile(keyPath, cert.PrivateKey, 0600); err != nil {
		return fmt.Errorf("failed to write private key file: %w", err)
	}

	m.logger.Info("Let's Encrypt certificate obtained successfully", "domain", m.domain)
	return nil
}

// ensureDir creates a directory if it doesn't exist
func ensureDir(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return os.MkdirAll(path, 0755)
	}
	return nil
}
