package config

import (
	"elasticetl/pkg/utils"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"gopkg.in/yaml.v2"
)

// Loader handles configuration loading and hot reloading
type Loader struct {
	configPath string
	config     *Config
	mutex      sync.RWMutex
	watcher    *fsnotify.Watcher
	callbacks  []func(*Config)
}

// NewLoader creates a new configuration loader
func NewLoader(configPath string) (*Loader, error) {
	loader := &Loader{
		configPath: configPath,
		callbacks:  make([]func(*Config), 0),
	}

	// Load initial configuration
	if err := loader.loadConfig(); err != nil {
		return nil, fmt.Errorf("failed to load initial config: %w", err)
	}

	// Setup file watcher for hot reload
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	loader.watcher = watcher

	// Add config file to watcher
	if err := watcher.Add(configPath); err != nil {
		return nil, fmt.Errorf("failed to watch config file: %w", err)
	}

	// Start watching for changes
	go loader.watchForChanges()

	return loader, nil
}

// GetConfig returns the current configuration (thread-safe)
func (l *Loader) GetConfig() *Config {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	return l.config
}

// OnConfigChange registers a callback for configuration changes
func (l *Loader) OnConfigChange(callback func(*Config)) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.callbacks = append(l.callbacks, callback)
}

// Close stops the configuration loader
func (l *Loader) Close() error {
	if l.watcher != nil {
		return l.watcher.Close()
	}
	return nil
}

// loadConfig loads configuration from file
func (l *Loader) loadConfig() error {
	data, err := ioutil.ReadFile(l.configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	ext := filepath.Ext(l.configPath)

	switch ext {
	case ".json":
		if err := json.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("failed to parse JSON config: %w", err)
		}
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("failed to parse YAML config: %w", err)
		}
	default:
		return fmt.Errorf("unsupported config file format: %s", ext)
	}

	// Validate configuration
	if err := l.validateConfig(&config); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	l.mutex.Lock()
	l.config = &config
	l.mutex.Unlock()

	return nil
}

// validateConfig validates the configuration
func (l *Loader) validateConfig(config *Config) error {
	if len(config.Pipelines) == 0 {
		return fmt.Errorf("at least one pipeline must be configured")
	}

	for i, pipeline := range config.Pipelines {
		if pipeline.Name == "" {
			return fmt.Errorf("pipeline %d: name is required", i)
		}

		if pipeline.Interval <= 0 {
			return fmt.Errorf("pipeline %s: interval must be positive", pipeline.Name)
		}

		if len(pipeline.Extract.URLs) == 0 {
			return fmt.Errorf("pipeline %s: at least one URL is required", pipeline.Name)
		}

		if len(pipeline.Extract.ClusterNames) == 0 {
			return fmt.Errorf("pipeline %s: at least one cluster name is required", pipeline.Name)
		}

		if pipeline.Extract.ElasticsearchQuery == "" {
			return fmt.Errorf("pipeline %s: elasticsearch query is required", pipeline.Name)
		}

		if len(pipeline.Load.Streams) == 0 {
			return fmt.Errorf("pipeline %s: at least one load stream is required", pipeline.Name)
		}

		// Validate conversion functions
		for j, conv := range pipeline.Transform.ConversionFunctions {
			if conv.Field == "" {
				return fmt.Errorf("pipeline %s: conversion function %d: field is required", pipeline.Name, j)
			}
			if conv.Function == "" {
				return fmt.Errorf("pipeline %s: conversion function %d: function is required", pipeline.Name, j)
			}
		}

		// Validate time expressions
		if err := utils.ValidateTimeExpression(pipeline.Extract.StartTime); err != nil {
			return fmt.Errorf("pipeline %s: invalid start_time: %w", pipeline.Name, err)
		}
		if err := utils.ValidateTimeExpression(pipeline.Extract.EndTime); err != nil {
			return fmt.Errorf("pipeline %s: invalid end_time: %w", pipeline.Name, err)
		}

		// Validate array lengths match
		minLen := len(pipeline.Extract.URLs)
		if len(pipeline.Extract.ClusterNames) < minLen {
			minLen = len(pipeline.Extract.ClusterNames)
		}
		if len(pipeline.Extract.AuthHeaders) > 0 && len(pipeline.Extract.AuthHeaders) < minLen {
			minLen = len(pipeline.Extract.AuthHeaders)
		}
		if len(pipeline.Extract.AdditionalHeaders) > 0 && len(pipeline.Extract.AdditionalHeaders) < minLen {
			minLen = len(pipeline.Extract.AdditionalHeaders)
		}

		if minLen == 0 {
			return fmt.Errorf("pipeline %s: no valid endpoint configurations found", pipeline.Name)
		}

		// Validate individual URLs and cluster names
		for j := 0; j < minLen; j++ {
			if pipeline.Extract.URLs[j] == "" {
				return fmt.Errorf("pipeline %s: URL %d is empty", pipeline.Name, j)
			}
			if pipeline.Extract.ClusterNames[j] == "" {
				return fmt.Errorf("pipeline %s: cluster_name %d is empty", pipeline.Name, j)
			}
		}
	}

	return nil
}

// watchForChanges watches for configuration file changes
func (l *Loader) watchForChanges() {
	for {
		select {
		case event, ok := <-l.watcher.Events:
			if !ok {
				return
			}

			if event.Op&fsnotify.Write == fsnotify.Write {
				// Wait a bit to ensure file write is complete
				time.Sleep(100 * time.Millisecond)

				if err := l.loadConfig(); err != nil {
					fmt.Printf("Failed to reload config: %v\n", err)
					continue
				}

				// Notify callbacks
				l.mutex.RLock()
				config := l.config
				callbacks := make([]func(*Config), len(l.callbacks))
				copy(callbacks, l.callbacks)
				l.mutex.RUnlock()

				for _, callback := range callbacks {
					go callback(config)
				}

				fmt.Println("Configuration reloaded successfully")
			}

		case err, ok := <-l.watcher.Errors:
			if !ok {
				return
			}
			fmt.Printf("Config watcher error: %v\n", err)
		}
	}
}
