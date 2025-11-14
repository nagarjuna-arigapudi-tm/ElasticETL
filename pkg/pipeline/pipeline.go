package pipeline

import (
	"context"
	"fmt"
	"sync"
	"time"

	"elasticetl/pkg/config"
	"elasticetl/pkg/extract"
	"elasticetl/pkg/load"
	"elasticetl/pkg/metrics"
	"elasticetl/pkg/transform"
)

// Pipeline represents a single ETL pipeline
type Pipeline struct {
	config          config.PipelineConfig
	extractor       *extract.Extractor
	transformer     *transform.Transformer
	loader          *load.Loader
	metrics         *metrics.Collector
	scheduler       *Scheduler
	retryTicker     *time.Ticker
	stopChan        chan struct{}
	mutex           sync.RWMutex
	running         bool
	failed          bool
	lastFailureTime time.Time
	// Add sync primitives for safe cleanup
	stopOnce      sync.Once
	goroutineDone chan struct{}
}

// NewPipeline creates a new pipeline
func NewPipeline(cfg config.PipelineConfig, metricsCollector *metrics.Collector) (*Pipeline, error) {
	// Create extractor
	extractor := extract.NewExtractor(cfg.Extract)

	// Create transformer
	transformer := transform.NewTransformer(cfg.Transform)

	// Create loader
	loader, err := load.NewLoader(cfg.Load)
	if err != nil {
		return nil, fmt.Errorf("failed to create loader: %w", err)
	}

	pipeline := &Pipeline{
		config:        cfg,
		extractor:     extractor,
		transformer:   transformer,
		loader:        loader,
		metrics:       metricsCollector,
		stopChan:      make(chan struct{}),
		goroutineDone: make(chan struct{}),
	}

	return pipeline, nil
}

// Start starts the pipeline
func (p *Pipeline) Start(ctx context.Context) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.running {
		return fmt.Errorf("pipeline %s is already running", p.config.Name)
	}

	if !p.config.Enabled {
		return fmt.Errorf("pipeline %s is disabled", p.config.Name)
	}

	p.running = true

	// Create scheduler with new schedule configuration
	p.scheduler = NewScheduler(p.config.Schedule, p.config.Interval)

	// Update metrics
	p.metrics.UpdatePipelineStatus(p.config.Name, true)

	// Start scheduler with execution function
	if err := p.scheduler.Start(ctx, func() {
		p.execute(ctx)
	}); err != nil {
		p.running = false
		return fmt.Errorf("failed to start scheduler: %w", err)
	}

	return nil
}

// shouldWriteExtractDebugOutput checks if extract debug output should be written
func (p *Pipeline) shouldWriteExtractDebugOutput() bool {
	return p.config.Extract.Debug.FinalQuery || p.config.Extract.Debug.APIResponse || p.config.Extract.Debug.FinalOutput
}

// writeExtractDebugOutput writes extract debug output with pipeline name
func (p *Pipeline) writeExtractDebugOutput(results []*extract.Result) error {
	return p.extractor.WriteDebugOutputWithPipelineName(results, p.config.Name)
}

// shouldWriteTransformDebugOutput checks if transform debug output should be written
func (p *Pipeline) shouldWriteTransformDebugOutput() bool {
	return p.config.Transform.Debug.Input || p.config.Transform.Debug.TransformedOutput || p.config.Transform.Debug.FinalOutput
}

// writeTransformDebugOutput writes transform debug output with pipeline name
func (p *Pipeline) writeTransformDebugOutput(inputResults []*extract.Result, transformResults []*transform.TransformedResult) error {
	return p.transformer.WriteDebugOutputWithPipelineName(inputResults, transformResults, p.config.Name)
}

// Stop stops the pipeline and waits for goroutines to finish
func (p *Pipeline) Stop() error {
	p.stopOnce.Do(func() {
		p.mutex.Lock()
		if !p.running {
			p.mutex.Unlock()
			return
		}

		p.running = false
		if p.scheduler != nil {
			p.scheduler.Stop()
		}

		// Signal goroutines to stop
		close(p.stopChan)
		p.mutex.Unlock()

		// Update metrics
		p.metrics.UpdatePipelineStatus(p.config.Name, false)
	})

	return nil
}

// IsRunning returns whether the pipeline is currently running
func (p *Pipeline) IsRunning() bool {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.running
}

// GetName returns the pipeline name
func (p *Pipeline) GetName() string {
	return p.config.Name
}

// UpdateConfig updates the pipeline configuration
func (p *Pipeline) UpdateConfig(cfg config.PipelineConfig) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	wasRunning := p.running

	// Stop if running
	if p.running {
		p.running = false
		if p.scheduler != nil {
			p.scheduler.Stop()
		}
		close(p.stopChan)
		p.stopChan = make(chan struct{})
	}

	// Update configuration
	p.config = cfg

	// Update components
	p.extractor.UpdateConfig(cfg.Extract)
	p.transformer.UpdateConfig(cfg.Transform)
	if err := p.loader.UpdateConfig(cfg.Load); err != nil {
		return fmt.Errorf("failed to update loader config: %w", err)
	}

	// Restart if it was running and still enabled
	if wasRunning && cfg.Enabled {
		p.running = true
		// Create new scheduler with updated configuration
		p.scheduler = NewScheduler(cfg.Schedule, cfg.Interval)
		// Start scheduler with execution function
		if err := p.scheduler.Start(context.Background(), func() {
			p.execute(context.Background())
		}); err != nil {
			p.running = false
			return fmt.Errorf("failed to start scheduler: %w", err)
		}
	}

	// Update metrics
	p.metrics.UpdatePipelineStatus(cfg.Name, cfg.Enabled && p.running)

	return nil
}

// execute performs a single ETL execution
func (p *Pipeline) execute(ctx context.Context) {
	startTime := time.Now()
	p.metrics.RecordPipelineStart(p.config.Name)

	// Wrap execution in a recovery function to ensure pipeline isolation
	defer func() {
		if r := recover(); r != nil {
			duration := time.Since(startTime)
			p.markAsFailed()
			p.metrics.RecordPipelineFailure(p.config.Name, duration, fmt.Errorf("pipeline panic: %v", r))
		}
	}()

	// Extract
	extractResults, err := p.extractor.Extract(ctx)
	if err != nil {
		duration := time.Since(startTime)
		p.markAsFailed()
		p.metrics.RecordPipelineFailure(p.config.Name, duration, fmt.Errorf("extraction failed: %w", err))
		return
	}

	// Write extract debug output with pipeline name if enabled
	if p.shouldWriteExtractDebugOutput() {
		if err := p.writeExtractDebugOutput(extractResults); err != nil {
			fmt.Printf("Failed to write extract debug output: %v\n", err)
		}
	}

	if len(extractResults) == 0 {
		// No data extracted, but not an error - clear failure state
		duration := time.Since(startTime)
		p.markAsSuccessful()
		p.metrics.RecordPipelineSuccess(p.config.Name, duration, 0, 0)
		return
	}

	// Transform
	transformResults, err := p.transformer.Transform(extractResults)
	if err != nil {
		duration := time.Since(startTime)
		p.markAsFailed()
		p.metrics.RecordPipelineFailure(p.config.Name, duration, fmt.Errorf("transformation failed: %w", err))
		return
	}

	// Write transform debug output with pipeline name if enabled
	if p.shouldWriteTransformDebugOutput() {
		if err := p.writeTransformDebugOutput(extractResults, transformResults); err != nil {
			fmt.Printf("Failed to write transform debug output: %v\n", err)
		}
	}

	// Load
	if err := p.loader.Load(ctx, transformResults, p.config.Name); err != nil {
		duration := time.Since(startTime)
		p.markAsFailed()
		p.metrics.RecordPipelineFailure(p.config.Name, duration, fmt.Errorf("loading failed: %w", err))
		return
	}

	// Calculate metrics and mark as successful
	duration := time.Since(startTime)
	entriesProcessed := int64(len(transformResults))
	bytesProcessed := p.calculateBytesProcessed(extractResults)

	p.markAsSuccessful()
	p.metrics.RecordPipelineSuccess(p.config.Name, duration, entriesProcessed, bytesProcessed)
}

// calculateBytesProcessed estimates the number of bytes processed
func (p *Pipeline) calculateBytesProcessed(results []*extract.Result) int64 {
	var totalBytes int64
	for _, result := range results {
		if responseSize, ok := result.Metadata["response_size"].(int); ok {
			totalBytes += int64(responseSize)
		}
	}
	return totalBytes
}

// markAsFailed marks the pipeline as failed and records the failure time
func (p *Pipeline) markAsFailed() {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.failed = true
	p.lastFailureTime = time.Now()
}

// markAsSuccessful marks the pipeline as successful and clears the failure state
func (p *Pipeline) markAsSuccessful() {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.failed = false
}

// IsFailed returns whether the pipeline is currently in a failed state
func (p *Pipeline) IsFailed() bool {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.failed
}

// GetLastFailureTime returns the time of the last failure
func (p *Pipeline) GetLastFailureTime() time.Time {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.lastFailureTime
}

// Close closes the pipeline and releases resources
func (p *Pipeline) Close() error {
	if err := p.Stop(); err != nil {
		return err
	}

	if err := p.loader.Close(); err != nil {
		return fmt.Errorf("failed to close loader: %w", err)
	}

	return nil
}

// Manager manages multiple pipelines
type Manager struct {
	pipelines map[string]*Pipeline
	metrics   *metrics.Collector
	mutex     sync.RWMutex
}

// NewManager creates a new pipeline manager
func NewManager(metricsCollector *metrics.Collector) *Manager {
	return &Manager{
		pipelines: make(map[string]*Pipeline),
		metrics:   metricsCollector,
	}
}

// AddPipeline adds a new pipeline
func (m *Manager) AddPipeline(cfg config.PipelineConfig) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if _, exists := m.pipelines[cfg.Name]; exists {
		return fmt.Errorf("pipeline %s already exists", cfg.Name)
	}

	pipeline, err := NewPipeline(cfg, m.metrics)
	if err != nil {
		return fmt.Errorf("failed to create pipeline %s: %w", cfg.Name, err)
	}

	m.pipelines[cfg.Name] = pipeline
	return nil
}

// RemovePipeline removes a pipeline
func (m *Manager) RemovePipeline(name string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	pipeline, exists := m.pipelines[name]
	if !exists {
		return fmt.Errorf("pipeline %s not found", name)
	}

	if err := pipeline.Close(); err != nil {
		return fmt.Errorf("failed to close pipeline %s: %w", name, err)
	}

	delete(m.pipelines, name)
	return nil
}

// StartPipeline starts a specific pipeline
func (m *Manager) StartPipeline(ctx context.Context, name string) error {
	m.mutex.RLock()
	pipeline, exists := m.pipelines[name]
	m.mutex.RUnlock()

	if !exists {
		return fmt.Errorf("pipeline %s not found", name)
	}

	return pipeline.Start(ctx)
}

// StopPipeline stops a specific pipeline
func (m *Manager) StopPipeline(name string) error {
	m.mutex.RLock()
	pipeline, exists := m.pipelines[name]
	m.mutex.RUnlock()

	if !exists {
		return fmt.Errorf("pipeline %s not found", name)
	}

	return pipeline.Stop()
}

// StartAllPipelines starts all enabled pipelines
func (m *Manager) StartAllPipelines(ctx context.Context) error {
	m.mutex.RLock()
	pipelines := make([]*Pipeline, 0, len(m.pipelines))
	for _, pipeline := range m.pipelines {
		pipelines = append(pipelines, pipeline)
	}
	m.mutex.RUnlock()

	var errors []error
	var successCount int

	for _, pipeline := range pipelines {
		if err := pipeline.Start(ctx); err != nil {
			errors = append(errors, fmt.Errorf("pipeline %s: %w", pipeline.GetName(), err))
		} else {
			successCount++
		}
	}

	// Log results but don't fail completely if some pipelines started successfully
	if len(errors) > 0 {
		if successCount > 0 {
			return fmt.Errorf("started %d pipelines successfully, but failed to start %d pipelines: %v", successCount, len(errors), errors)
		}
		return fmt.Errorf("failed to start all pipelines: %v", errors)
	}

	return nil
}

// StopAllPipelines stops all pipelines
func (m *Manager) StopAllPipelines() error {
	m.mutex.RLock()
	pipelines := make([]*Pipeline, 0, len(m.pipelines))
	for _, pipeline := range m.pipelines {
		pipelines = append(pipelines, pipeline)
	}
	m.mutex.RUnlock()

	var errors []error
	for _, pipeline := range pipelines {
		if err := pipeline.Stop(); err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to stop some pipelines: %v", errors)
	}

	return nil
}

// UpdatePipelines updates pipelines based on new configuration
func (m *Manager) UpdatePipelines(configs []config.PipelineConfig) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Create a map of new configurations
	newConfigs := make(map[string]config.PipelineConfig)
	for _, cfg := range configs {
		newConfigs[cfg.Name] = cfg
	}

	// Update existing pipelines or remove if not in new config
	for name, pipeline := range m.pipelines {
		if newCfg, exists := newConfigs[name]; exists {
			if err := pipeline.UpdateConfig(newCfg); err != nil {
				return fmt.Errorf("failed to update pipeline %s: %w", name, err)
			}
			delete(newConfigs, name) // Remove from new configs as it's been processed
		} else {
			// Pipeline no longer exists in config, remove it
			if err := pipeline.Close(); err != nil {
				return fmt.Errorf("failed to close pipeline %s: %w", name, err)
			}
			delete(m.pipelines, name)
		}
	}

	// Add new pipelines
	for _, cfg := range newConfigs {
		pipeline, err := NewPipeline(cfg, m.metrics)
		if err != nil {
			return fmt.Errorf("failed to create new pipeline %s: %w", cfg.Name, err)
		}
		m.pipelines[cfg.Name] = pipeline

		// Start if enabled
		if cfg.Enabled {
			if err := pipeline.Start(context.Background()); err != nil {
				return fmt.Errorf("failed to start new pipeline %s: %w", cfg.Name, err)
			}
		}
	}

	return nil
}

// PipelineStatus represents detailed status information for a pipeline
type PipelineStatus struct {
	Name            string    `json:"name"`
	Running         bool      `json:"running"`
	Enabled         bool      `json:"enabled"`
	Failed          bool      `json:"failed"`
	LastFailureTime time.Time `json:"last_failure_time,omitempty"`
	Interval        string    `json:"interval"`
	RetryInterval   string    `json:"retry_interval,omitempty"`
}

// GetPipelineStatus returns the status of all pipelines
func (m *Manager) GetPipelineStatus() map[string]bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	status := make(map[string]bool)
	for name, pipeline := range m.pipelines {
		status[name] = pipeline.IsRunning()
	}

	return status
}

// GetDetailedPipelineStatus returns detailed status information for all pipelines
func (m *Manager) GetDetailedPipelineStatus() map[string]PipelineStatus {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	status := make(map[string]PipelineStatus)
	for name, pipeline := range m.pipelines {
		// Determine schedule display string
		scheduleStr := ""
		if pipeline.config.Schedule.CronSchedule != "" {
			scheduleStr = fmt.Sprintf("cron: %s", pipeline.config.Schedule.CronSchedule)
		} else if pipeline.config.Schedule.Interval > 0 {
			scheduleStr = fmt.Sprintf("interval: %s", pipeline.config.Schedule.Interval.String())
		} else {
			scheduleStr = "not configured"
		}

		pipelineStatus := PipelineStatus{
			Name:     name,
			Running:  pipeline.IsRunning(),
			Enabled:  pipeline.config.Enabled,
			Failed:   pipeline.IsFailed(),
			Interval: scheduleStr,
		}

		if pipeline.config.RetryInterval > 0 {
			pipelineStatus.RetryInterval = pipeline.config.RetryInterval.String()
		}

		if pipeline.IsFailed() {
			pipelineStatus.LastFailureTime = pipeline.GetLastFailureTime()
		}

		status[name] = pipelineStatus
	}

	return status
}

// Close closes all pipelines and releases resources
func (m *Manager) Close() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	var errors []error
	for name, pipeline := range m.pipelines {
		if err := pipeline.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close pipeline %s: %w", name, err))
		}
	}

	m.pipelines = make(map[string]*Pipeline)

	if len(errors) > 0 {
		return fmt.Errorf("failed to close some pipelines: %v", errors)
	}

	return nil
}
