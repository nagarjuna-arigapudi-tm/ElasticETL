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
	config      config.PipelineConfig
	extractor   *extract.Extractor
	transformer *transform.Transformer
	loader      *load.Loader
	metrics     *metrics.Collector
	ticker      *time.Ticker
	stopChan    chan struct{}
	mutex       sync.RWMutex
	running     bool
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
		config:      cfg,
		extractor:   extractor,
		transformer: transformer,
		loader:      loader,
		metrics:     metricsCollector,
		stopChan:    make(chan struct{}),
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
	p.ticker = time.NewTicker(p.config.Interval)

	// Update metrics
	p.metrics.UpdatePipelineStatus(p.config.Name, true)

	// Start pipeline execution loop
	go p.run(ctx)

	return nil
}

// Stop stops the pipeline
func (p *Pipeline) Stop() error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if !p.running {
		return nil
	}

	p.running = false
	if p.ticker != nil {
		p.ticker.Stop()
	}

	close(p.stopChan)

	// Update metrics
	p.metrics.UpdatePipelineStatus(p.config.Name, false)

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
		if p.ticker != nil {
			p.ticker.Stop()
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
		p.ticker = time.NewTicker(cfg.Interval)
		go p.run(context.Background()) // Use background context for restart
	}

	// Update metrics
	p.metrics.UpdatePipelineStatus(cfg.Name, cfg.Enabled && p.running)

	return nil
}

// run executes the pipeline loop
func (p *Pipeline) run(ctx context.Context) {
	defer func() {
		p.mutex.Lock()
		p.running = false
		p.mutex.Unlock()
	}()

	// Execute immediately on start
	p.execute(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-p.stopChan:
			return
		case <-p.ticker.C:
			p.execute(ctx)
		}
	}
}

// execute performs a single ETL execution
func (p *Pipeline) execute(ctx context.Context) {
	startTime := time.Now()
	p.metrics.RecordPipelineStart(p.config.Name)

	// Extract
	extractResults, err := p.extractor.Extract(ctx)
	if err != nil {
		duration := time.Since(startTime)
		p.metrics.RecordPipelineFailure(p.config.Name, duration, fmt.Errorf("extraction failed: %w", err))
		return
	}

	if len(extractResults) == 0 {
		// No data extracted, but not an error
		duration := time.Since(startTime)
		p.metrics.RecordPipelineSuccess(p.config.Name, duration, 0, 0)
		return
	}

	// Transform
	transformResults, err := p.transformer.Transform(extractResults)
	if err != nil {
		duration := time.Since(startTime)
		p.metrics.RecordPipelineFailure(p.config.Name, duration, fmt.Errorf("transformation failed: %w", err))
		return
	}

	// Load
	if err := p.loader.Load(ctx, transformResults); err != nil {
		duration := time.Since(startTime)
		p.metrics.RecordPipelineFailure(p.config.Name, duration, fmt.Errorf("loading failed: %w", err))
		return
	}

	// Calculate metrics
	duration := time.Since(startTime)
	entriesProcessed := int64(len(transformResults))
	bytesProcessed := p.calculateBytesProcessed(extractResults)

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
	for _, pipeline := range pipelines {
		if err := pipeline.Start(ctx); err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to start some pipelines: %v", errors)
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
