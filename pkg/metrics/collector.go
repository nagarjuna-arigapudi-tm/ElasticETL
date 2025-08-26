package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"sync"
	"time"

	"elasticetl/pkg/config"
)

// PipelineMetrics represents metrics for a single pipeline
type PipelineMetrics struct {
	Name               string        `json:"name"`
	Enabled            bool          `json:"enabled"`
	LastRun            time.Time     `json:"last_run"`
	LastDuration       time.Duration `json:"last_duration"`
	TotalRuns          int64         `json:"total_runs"`
	SuccessfulRuns     int64         `json:"successful_runs"`
	FailedRuns         int64         `json:"failed_runs"`
	EntriesProcessed   int64         `json:"entries_processed"`
	BytesProcessed     int64         `json:"bytes_processed"`
	MemoryUsageMB      float64       `json:"memory_usage_mb"`
	CPUUsagePercent    float64       `json:"cpu_usage_percent"`
	ActiveGoroutines   int           `json:"active_goroutines"`
	ErrorRate          float64       `json:"error_rate"`
	AverageProcessTime time.Duration `json:"average_process_time"`
	LastError          string        `json:"last_error,omitempty"`
	LastErrorTime      time.Time     `json:"last_error_time,omitempty"`
}

// SystemMetrics represents overall system metrics
type SystemMetrics struct {
	TotalMemoryMB    float64       `json:"total_memory_mb"`
	UsedMemoryMB     float64       `json:"used_memory_mb"`
	CPUUsagePercent  float64       `json:"cpu_usage_percent"`
	TotalGoroutines  int           `json:"total_goroutines"`
	ActivePipelines  int           `json:"active_pipelines"`
	TotalPipelines   int           `json:"total_pipelines"`
	Uptime           time.Duration `json:"uptime"`
	LastConfigReload time.Time     `json:"last_config_reload"`
}

// Collector handles metrics collection and reporting
type Collector struct {
	config          config.MetricsConfig
	pipelineMetrics map[string]*PipelineMetrics
	systemMetrics   *SystemMetrics
	mutex           sync.RWMutex
	startTime       time.Time
	httpServer      *http.Server
}

// NewCollector creates a new metrics collector
func NewCollector(cfg config.MetricsConfig) *Collector {
	collector := &Collector{
		config:          cfg,
		pipelineMetrics: make(map[string]*PipelineMetrics),
		systemMetrics:   &SystemMetrics{},
		startTime:       time.Now(),
	}

	if cfg.Enabled {
		collector.startHTTPServer()
		go collector.collectSystemMetrics()
	}

	return collector
}

// RecordPipelineStart records the start of a pipeline execution
func (c *Collector) RecordPipelineStart(pipelineName string) {
	if !c.config.Enabled {
		return
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	metrics, exists := c.pipelineMetrics[pipelineName]
	if !exists {
		metrics = &PipelineMetrics{
			Name:    pipelineName,
			Enabled: true,
		}
		c.pipelineMetrics[pipelineName] = metrics
	}

	metrics.LastRun = time.Now()
	metrics.TotalRuns++
}

// RecordPipelineSuccess records a successful pipeline execution
func (c *Collector) RecordPipelineSuccess(pipelineName string, duration time.Duration, entriesProcessed int64, bytesProcessed int64) {
	if !c.config.Enabled {
		return
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	metrics, exists := c.pipelineMetrics[pipelineName]
	if !exists {
		return
	}

	metrics.LastDuration = duration
	metrics.SuccessfulRuns++
	metrics.EntriesProcessed += entriesProcessed
	metrics.BytesProcessed += bytesProcessed

	// Calculate average process time
	if metrics.SuccessfulRuns > 0 {
		totalTime := time.Duration(metrics.SuccessfulRuns)*metrics.AverageProcessTime + duration
		metrics.AverageProcessTime = totalTime / time.Duration(metrics.SuccessfulRuns)
	} else {
		metrics.AverageProcessTime = duration
	}

	// Calculate error rate
	if metrics.TotalRuns > 0 {
		metrics.ErrorRate = float64(metrics.FailedRuns) / float64(metrics.TotalRuns) * 100
	}
}

// RecordPipelineFailure records a failed pipeline execution
func (c *Collector) RecordPipelineFailure(pipelineName string, duration time.Duration, err error) {
	if !c.config.Enabled {
		return
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	metrics, exists := c.pipelineMetrics[pipelineName]
	if !exists {
		return
	}

	metrics.LastDuration = duration
	metrics.FailedRuns++
	metrics.LastError = err.Error()
	metrics.LastErrorTime = time.Now()

	// Calculate error rate
	if metrics.TotalRuns > 0 {
		metrics.ErrorRate = float64(metrics.FailedRuns) / float64(metrics.TotalRuns) * 100
	}
}

// UpdatePipelineStatus updates the enabled status of a pipeline
func (c *Collector) UpdatePipelineStatus(pipelineName string, enabled bool) {
	if !c.config.Enabled {
		return
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	metrics, exists := c.pipelineMetrics[pipelineName]
	if !exists {
		metrics = &PipelineMetrics{
			Name:    pipelineName,
			Enabled: enabled,
		}
		c.pipelineMetrics[pipelineName] = metrics
	} else {
		metrics.Enabled = enabled
	}
}

// RecordConfigReload records a configuration reload event
func (c *Collector) RecordConfigReload() {
	if !c.config.Enabled {
		return
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.systemMetrics.LastConfigReload = time.Now()
}

// GetPipelineMetrics returns metrics for a specific pipeline
func (c *Collector) GetPipelineMetrics(pipelineName string) *PipelineMetrics {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if metrics, exists := c.pipelineMetrics[pipelineName]; exists {
		// Return a copy to prevent external modification
		metricsCopy := *metrics
		return &metricsCopy
	}

	return nil
}

// GetAllPipelineMetrics returns metrics for all pipelines
func (c *Collector) GetAllPipelineMetrics() map[string]*PipelineMetrics {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	result := make(map[string]*PipelineMetrics)
	for name, metrics := range c.pipelineMetrics {
		metricsCopy := *metrics
		result[name] = &metricsCopy
	}

	return result
}

// GetSystemMetrics returns current system metrics
func (c *Collector) GetSystemMetrics() *SystemMetrics {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	// Return a copy to prevent external modification
	metricsCopy := *c.systemMetrics
	metricsCopy.Uptime = time.Since(c.startTime)
	metricsCopy.TotalPipelines = len(c.pipelineMetrics)

	activePipelines := 0
	for _, metrics := range c.pipelineMetrics {
		if metrics.Enabled {
			activePipelines++
		}
	}
	metricsCopy.ActivePipelines = activePipelines

	return &metricsCopy
}

// collectSystemMetrics periodically collects system-level metrics
func (c *Collector) collectSystemMetrics() {
	ticker := time.NewTicker(c.config.Interval)
	defer ticker.Stop()

	for range ticker.C {
		c.updateSystemMetrics()
	}
}

// updateSystemMetrics updates system-level metrics
func (c *Collector) updateSystemMetrics() {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.systemMetrics.TotalMemoryMB = float64(memStats.Sys) / 1024 / 1024
	c.systemMetrics.UsedMemoryMB = float64(memStats.Alloc) / 1024 / 1024
	c.systemMetrics.TotalGoroutines = runtime.NumGoroutine()

	// Update individual pipeline memory usage (simplified)
	for _, metrics := range c.pipelineMetrics {
		metrics.MemoryUsageMB = c.systemMetrics.UsedMemoryMB / float64(len(c.pipelineMetrics))
		metrics.ActiveGoroutines = c.systemMetrics.TotalGoroutines / len(c.pipelineMetrics)
	}
}

// startHTTPServer starts the HTTP server for metrics endpoint
func (c *Collector) startHTTPServer() {
	mux := http.NewServeMux()
	mux.HandleFunc(c.config.Path, c.handleMetricsRequest)
	mux.HandleFunc(c.config.Path+"/pipeline/", c.handlePipelineMetricsRequest)
	mux.HandleFunc(c.config.Path+"/system", c.handleSystemMetricsRequest)

	c.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", c.config.Port),
		Handler: mux,
	}

	go func() {
		if err := c.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("Metrics server error: %v\n", err)
		}
	}()
}

// handleMetricsRequest handles requests for all metrics
func (c *Collector) handleMetricsRequest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	response := map[string]interface{}{
		"system":    c.GetSystemMetrics(),
		"pipelines": c.GetAllPipelineMetrics(),
	}

	if err := writeJSONResponse(w, response); err != nil {
		http.Error(w, "Failed to encode metrics", http.StatusInternalServerError)
	}
}

// handlePipelineMetricsRequest handles requests for specific pipeline metrics
func (c *Collector) handlePipelineMetricsRequest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Extract pipeline name from URL path
	pipelineName := r.URL.Path[len(c.config.Path+"/pipeline/"):]
	if pipelineName == "" {
		http.Error(w, "Pipeline name required", http.StatusBadRequest)
		return
	}

	metrics := c.GetPipelineMetrics(pipelineName)
	if metrics == nil {
		http.Error(w, "Pipeline not found", http.StatusNotFound)
		return
	}

	if err := writeJSONResponse(w, metrics); err != nil {
		http.Error(w, "Failed to encode metrics", http.StatusInternalServerError)
	}
}

// handleSystemMetricsRequest handles requests for system metrics
func (c *Collector) handleSystemMetricsRequest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if err := writeJSONResponse(w, c.GetSystemMetrics()); err != nil {
		http.Error(w, "Failed to encode metrics", http.StatusInternalServerError)
	}
}

// writeJSONResponse writes a JSON response
func writeJSONResponse(w http.ResponseWriter, data interface{}) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

// Close stops the metrics collector
func (c *Collector) Close() error {
	if c.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return c.httpServer.Shutdown(ctx)
	}
	return nil
}

// UpdateConfig updates the metrics collector configuration
func (c *Collector) UpdateConfig(cfg config.MetricsConfig) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// If metrics were disabled and now enabled, start server
	if !c.config.Enabled && cfg.Enabled {
		c.config = cfg
		c.startHTTPServer()
		go c.collectSystemMetrics()
		return nil
	}

	// If metrics were enabled and now disabled, stop server
	if c.config.Enabled && !cfg.Enabled {
		if c.httpServer != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			c.httpServer.Shutdown(ctx)
		}
	}

	c.config = cfg
	return nil
}
