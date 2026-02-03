package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/apardo/firecracker-agent/internal/agent"
	"github.com/apardo/firecracker-agent/internal/monitor"
	"github.com/apardo/firecracker-agent/pkg/config"
	"github.com/apardo/firecracker-agent/pkg/logger"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
)

const version = "0.1.0"

var (
	cfgFile   string
	log       *logrus.Logger
	startTime time.Time
)

func main() {
	startTime = time.Now()

	rootCmd := &cobra.Command{
		Use:     "fc-agent",
		Short:   "Firecracker Agent - gRPC VM management service",
		Long:    `High-performance gRPC service for managing Firecracker microVMs`,
		Version: version,
		RunE:    run,
	}

	rootCmd.Flags().StringVar(&cfgFile, "config", "configs/agent.yaml", "config file path")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	// Load config
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Setup logger
	log = logger.New(cfg.Log.Level, cfg.Log.Format)
	log.WithFields(logrus.Fields{
		"version": version,
		"config":  cfgFile,
	}).Info("Starting Firecracker Agent")

	// Create gRPC server with interceptors
	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(agent.LoggingInterceptor(log)),
	)

	// Create agent server
	agentServer, err := agent.NewServer(cfg, log, startTime)
	if err != nil {
		return fmt.Errorf("failed to create agent server: %w", err)
	}

	// Register gRPC service
	agentServer.Register(grpcServer)

	// Start metrics server if enabled
	if cfg.Monitoring.Enabled {
		metricsServer := monitor.NewMetricsServer(cfg.Monitoring.MetricsPort, log)
		go func() {
			log.WithField("port", cfg.Monitoring.MetricsPort).Info("Starting metrics server")
			if err := metricsServer.Start(); err != nil {
				log.WithError(err).Error("Metrics server failed")
			}
		}()
	}

	// Start gRPC server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	log.WithField("address", addr).Info("gRPC server listening")

	// Start server in goroutine
	errChan := make(chan error, 1)
	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			errChan <- err
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errChan:
		return fmt.Errorf("server error: %w", err)
	case sig := <-sigChan:
		log.WithField("signal", sig).Info("Received shutdown signal")

		// Graceful shutdown
		log.Info("Shutting down gracefully...")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Stop gRPC server
		stopped := make(chan struct{})
		go func() {
			grpcServer.GracefulStop()
			close(stopped)
		}()

		select {
		case <-stopped:
			log.Info("Server stopped gracefully")
		case <-ctx.Done():
			log.Warn("Shutdown timeout, forcing stop")
			grpcServer.Stop()
		}

		return nil
	}
}
