package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"elasticetl/pkg/config"
	"elasticetl/pkg/metrics"
	"elasticetl/pkg/pipeline"
)

const (
	defaultConfigPath = "configs/config.json"
	defaultLogLevel   = "info"
)

func main() {
	// Parse command line flags
	var (
		configPath = flag.String("config", defaultConfigPath, "Path to configuration file")
		logLevel   = flag.String("log-level", defaultLogLevel, "Log level (debug, info, warn, error)")
		version    = flag.Bool("version", false, "Show version information")
	)
	flag.Parse()

	if *version {
		fmt.Println("ElasticETL v1.0.0")
		fmt.Println("A flexible ETL tool for Elasticsearch data processing")
		os.Exit(0)
	}

	// Setup logging
	setupLogging(*logLevel)

	log.Printf("Starting ElasticETL with config: %s", *configPath)

	// Load configuration
	configLoader, err := config.NewLoader(*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}
	defer configLoader.Close()

	initialConfig := configLoader.GetConfig()

	// Initialize metrics collector
	metricsCollector := metrics.NewCollector(initialConfig.Global.Metrics)
	defer metricsCollector.Close()

	// Initialize pipeline manager
	pipelineManager := pipeline.NewManager(metricsCollector)
	defer pipelineManager.Close()

	// Create initial pipelines
	for _, pipelineCfg := range initialConfig.Pipelines {
		if err := pipelineManager.AddPipeline(pipelineCfg); err != nil {
			log.Fatalf("Failed to add pipeline %s: %v", pipelineCfg.Name, err)
		}
	}

	// Setup configuration hot reload
	configLoader.OnConfigChange(func(newConfig *config.Config) {
		log.Println("Configuration changed, updating pipelines...")

		// Update metrics collector
		if err := metricsCollector.UpdateConfig(newConfig.Global.Metrics); err != nil {
			log.Printf("Failed to update metrics config: %v", err)
		}

		// Update pipelines
		if err := pipelineManager.UpdatePipelines(newConfig.Pipelines); err != nil {
			log.Printf("Failed to update pipelines: %v", err)
		} else {
			log.Println("Pipelines updated successfully")
			metricsCollector.RecordConfigReload()
		}
	})

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start all enabled pipelines
	if err := pipelineManager.StartAllPipelines(ctx); err != nil {
		log.Printf("Warning: Failed to start some pipelines: %v", err)
	}

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	log.Println("ElasticETL started successfully")
	log.Printf("Metrics available at http://localhost:%d%s",
		initialConfig.Global.Metrics.Port,
		initialConfig.Global.Metrics.Path)

	// Print pipeline status
	printPipelineStatus(pipelineManager)

	// Wait for shutdown signal
	<-sigChan
	log.Println("Shutdown signal received, stopping ElasticETL...")

	// Cancel context to stop all operations
	cancel()

	// Stop all pipelines with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	done := make(chan error, 1)
	go func() {
		done <- pipelineManager.StopAllPipelines()
	}()

	select {
	case err := <-done:
		if err != nil {
			log.Printf("Error stopping pipelines: %v", err)
		} else {
			log.Println("All pipelines stopped successfully")
		}
	case <-shutdownCtx.Done():
		log.Println("Shutdown timeout reached, forcing exit")
	}

	log.Println("ElasticETL stopped")
}

// setupLogging configures logging based on the specified level
func setupLogging(level string) {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	switch level {
	case "debug":
		log.SetOutput(os.Stdout)
	case "info":
		log.SetOutput(os.Stdout)
	case "warn":
		log.SetOutput(os.Stderr)
	case "error":
		log.SetOutput(os.Stderr)
	default:
		log.SetOutput(os.Stdout)
	}
}

// printPipelineStatus prints the current status of all pipelines
func printPipelineStatus(manager *pipeline.Manager) {
	status := manager.GetPipelineStatus()
	if len(status) == 0 {
		log.Println("No pipelines configured")
		return
	}

	log.Println("Pipeline Status:")
	for name, running := range status {
		statusStr := "stopped"
		if running {
			statusStr = "running"
		}
		log.Printf("  %s: %s", name, statusStr)
	}
}
