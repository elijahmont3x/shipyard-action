package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/elijahmont3x/shipyard-action/pkg/config"
	"github.com/elijahmont3x/shipyard-action/pkg/deployment"
	"github.com/elijahmont3x/shipyard-action/pkg/log"
)

func main() {
	logger := log.NewLogger("shipyard")
	logger.Info("Starting Shipyard deployment")

	// Setup signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-signalCh
		logger.Info(fmt.Sprintf("Received signal %s, shutting down...", sig))
		cancel()
		// Allow some time for cleanup before forcing exit
		time.Sleep(5 * time.Second)
		os.Exit(1)
	}()

	// Load configuration from GitHub Actions inputs and config file
	cfg, err := config.Load()
	if err != nil {
		logger.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Create and start the deployment manager
	manager, err := deployment.NewManager(cfg, logger)
	if err != nil {
		logger.Error("Failed to create deployment manager", "error", err)
		os.Exit(1)
	}

	// Run the deployment process
	if err := manager.Deploy(ctx); err != nil {
		logger.Error("Deployment failed", "error", err)
		// Attempt rollback if deployment fails
		logger.Info("Attempting rollback")
		if rbErr := manager.Rollback(ctx); rbErr != nil {
			logger.Error("Rollback failed", "error", rbErr)
		}
		os.Exit(1)
	}

	// Add external verification
	logger.Info("Deployment successful, performing external verification")
	if err := manager.VerifyExternalAccess(ctx); err != nil {
		logger.Warn("External verification failed, but deployment was successful", "error", err)
		// We don't exit with error here since the deployment itself was successful
		// This just provides additional verification
	} else {
		logger.Info("External verification successful - Your application is accessible from the internet! 🚀")
	}

	logger.Info("Deployment completed successfully")
}
