package load

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"elasticetl/pkg/config"
	"elasticetl/pkg/transform"
)

// substituteEnvVars replaces environment variables in the format ${VAR_NAME}
func substituteEnvVars(input string) string {
	re := regexp.MustCompile(`\$\{([^}]+)\}`)
	return re.ReplaceAllStringFunc(input, func(match string) string {
		// Extract variable name from ${VAR_NAME}
		varName := strings.TrimPrefix(strings.TrimSuffix(match, "}"), "${")
		if envValue := os.Getenv(varName); envValue != "" {
			return envValue
		}
		return match // Return original if env var not found
	})
}

// createBasicAuthHeader creates a basic auth header from username and password
func createBasicAuthHeader(username, password string) string {
	// Substitute environment variables
	username = substituteEnvVars(username)
	password = substituteEnvVars(password)

	// Create basic auth header
	auth := username + ":" + password
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(auth))
}

// parseBasicAuth parses basic auth configuration from stream config
func parseBasicAuth(config map[string]interface{}) (string, error) {
	basicAuthRaw, ok := config["basic_auth"]
	if !ok {
		return "", nil // No basic auth configured
	}

	basicAuthMap, ok := basicAuthRaw.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("basic_auth must be an object")
	}

	username, ok := basicAuthMap["username"].(string)
	if !ok {
		return "", fmt.Errorf("basic_auth.username is required")
	}

	password, ok := basicAuthMap["password"].(string)
	if !ok {
		return "", fmt.Errorf("basic_auth.password is required")
	}

	return createBasicAuthHeader(username, password), nil
}

// Loader handles data loading to various destinations
type Loader struct {
	config  config.LoadConfig
	streams []Stream
	mutex   sync.RWMutex
}

// Stream interface for different load destinations
type Stream interface {
	Load(ctx context.Context, results []*transform.TransformedResult) error
	Close() error
	GetType() string
}

// NewLoader creates a new loader
func NewLoader(cfg config.LoadConfig) (*Loader, error) {
	loader := &Loader{
		config: cfg,
	}

	// Initialize streams
	for _, streamCfg := range cfg.Streams {
		stream, err := createStream(streamCfg, cfg.Metrics)
		if err != nil {
			return nil, fmt.Errorf("failed to create stream %s: %w", streamCfg.Type, err)
		}
		loader.streams = append(loader.streams, stream)
	}

	return loader, nil
}

// Load loads data to all configured streams
func (l *Loader) Load(ctx context.Context, results []*transform.TransformedResult) error {
	l.mutex.RLock()
	streams := make([]Stream, len(l.streams))
	copy(streams, l.streams)
	l.mutex.RUnlock()

	var wg sync.WaitGroup
	errorsChan := make(chan error, len(streams))

	// Load to all streams concurrently
	for _, stream := range streams {
		wg.Add(1)
		go func(s Stream) {
			defer wg.Done()
			if err := s.Load(ctx, results); err != nil {
				errorsChan <- fmt.Errorf("stream %s: %w", s.GetType(), err)
			}
		}(stream)
	}

	// Wait for all loads to complete
	go func() {
		wg.Wait()
		close(errorsChan)
	}()

	// Collect errors
	var errors []error
	for err := range errorsChan {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		return fmt.Errorf("load errors: %v", errors)
	}

	return nil
}

// Close closes all streams
func (l *Loader) Close() error {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	var errors []error
	for _, stream := range l.streams {
		if err := stream.Close(); err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("close errors: %v", errors)
	}

	return nil
}

// UpdateConfig updates the loader configuration
func (l *Loader) UpdateConfig(cfg config.LoadConfig) error {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	// Close existing streams
	for _, stream := range l.streams {
		stream.Close()
	}

	// Create new streams
	l.streams = nil
	for _, streamCfg := range cfg.Streams {
		stream, err := createStream(streamCfg, cfg.Metrics)
		if err != nil {
			return fmt.Errorf("failed to create stream %s: %w", streamCfg.Type, err)
		}
		l.streams = append(l.streams, stream)
	}

	l.config = cfg
	return nil
}

// createStream creates a stream based on configuration
func createStream(cfg config.StreamConfig, metrics []config.PrometheusMetricConfig) (Stream, error) {
	switch cfg.Type {
	case "gem":
		return NewGEMStream(cfg.Config, cfg.Labels, cfg.InsecureTLS, metrics)
	case "otel":
		return NewOTELStream(cfg.Config, cfg.Labels, cfg.InsecureTLS, metrics)
	case "prometheus":
		return NewPrometheusStream(cfg.Config, cfg.Labels, cfg.InsecureTLS, metrics)
	case "debug":
		return NewDebugStream(cfg.Config, metrics)
	case "csv":
		return NewCSVStream(cfg.Config)
	default:
		return nil, fmt.Errorf("unsupported stream type: %s", cfg.Type)
	}
}

// GEMStream handles loading to GEM with Prometheus remote write
type GEMStream struct {
	endpoint   string
	httpClient *http.Client
	labels     map[string]string
	metrics    []config.PrometheusMetricConfig
}

// NewGEMStream creates a new GEM stream
func NewGEMStream(config map[string]interface{}, labels map[string]string, insecureTLS bool, metrics []config.PrometheusMetricConfig) (*GEMStream, error) {
	endpoint, ok := config["endpoint"].(string)
	if !ok {
		return nil, fmt.Errorf("gem stream requires 'endpoint' configuration")
	}

	timeout := 30 * time.Second
	if t, ok := config["timeout"].(string); ok {
		if parsed, err := time.ParseDuration(t); err == nil {
			timeout = parsed
		}
	}

	// Configure HTTP client with TLS settings
	transport := &http.Transport{}
	if insecureTLS {
		transport.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	}

	return &GEMStream{
		endpoint: endpoint,
		labels:   labels,
		metrics:  metrics,
		httpClient: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
	}, nil
}

// Load loads data to GEM
func (g *GEMStream) Load(ctx context.Context, results []*transform.TransformedResult) error {
	// Convert results to Prometheus remote write format
	samples := g.convertToPrometheusSamples(results)
	if len(samples) == 0 {
		return nil
	}

	// Create remote write request
	writeRequest := map[string]interface{}{
		"timeseries": samples,
	}

	jsonData, err := json.Marshal(writeRequest)
	if err != nil {
		return fmt.Errorf("failed to marshal prometheus data: %w", err)
	}

	// Send to GEM endpoint
	req, err := http.NewRequestWithContext(ctx, "POST", g.endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Prometheus-Remote-Write-Version", "0.1.0")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("GEM returned status %d", resp.StatusCode)
	}

	return nil
}

// convertToPrometheusSamples converts transformed results to Prometheus samples using CSV data
func (g *GEMStream) convertToPrometheusSamples(results []*transform.TransformedResult) []map[string]interface{} {
	var samples []map[string]interface{}

	for _, result := range results {
		// Use CSV data to create time series if available and metrics are configured
		if len(result.CSVData) > 0 && len(g.metrics) > 0 {
			// Generate time series for each metric using CSV data
			for _, metric := range g.metrics {
				metricSamples := g.createPrometheusTimeSeriesForMetric(result.CSVData, metric)
				samples = append(samples, metricSamples...)
			}
			continue
		}

		// Fallback to old behavior using TransformedData
		timestamp := result.Timestamp.UnixMilli()
		for key, value := range result.TransformedData {
			// Only include numeric values as metrics
			if numValue, ok := g.toFloat64(value); ok {
				// Create labels map starting with metric name and source
				labels := map[string]string{
					"__name__": key,
					"source":   result.Source,
				}

				// Add cluster name from metadata if available
				if clusterName, ok := result.Metadata["cluster_name"].(string); ok && clusterName != "" {
					labels["cluster"] = clusterName
				}

				// Add configured labels
				for labelKey, labelValue := range g.labels {
					labels[labelKey] = labelValue
				}

				sample := map[string]interface{}{
					"labels": []map[string]string{labels},
					"samples": []map[string]interface{}{
						{
							"value":     numValue,
							"timestamp": timestamp,
						},
					},
				}
				samples = append(samples, sample)
			}
		}
	}

	return samples
}

// createPrometheusTimeSeriesForMetric creates Prometheus remote write time series for a specific metric
func (g *GEMStream) createPrometheusTimeSeriesForMetric(csvData [][]string, metric config.PrometheusMetricConfig) []map[string]interface{} {
	var samples []map[string]interface{}

	// Group CSV rows by unique field combinations
	uniqueGroups := make(map[string][]map[string]interface{})

	for _, row := range csvData {
		if len(row) <= metric.Value || len(row) <= metric.Timestamp {
			continue // Skip rows that don't have required columns
		}

		// Create unique key from uniqueFieldsIndex
		var keyParts []string
		for _, idx := range metric.UniqueFieldsIndex {
			if idx < len(row) {
				keyParts = append(keyParts, row[idx])
			}
		}
		uniqueKey := strings.Join(keyParts, "|")

		// Parse value and timestamp
		value, ok := g.parseFloat(row[metric.Value])
		if !ok {
			continue
		}

		timestamp, ok := g.parseInt64(row[metric.Timestamp])
		if !ok {
			continue
		}

		// Create sample
		sample := map[string]interface{}{
			"value":     value,
			"timestamp": timestamp,
			"row":       row,
		}

		uniqueGroups[uniqueKey] = append(uniqueGroups[uniqueKey], sample)
	}

	// Generate time series for each unique group
	for _, groupSamples := range uniqueGroups {
		if len(groupSamples) == 0 {
			continue
		}

		// Build labels from first sample in group
		firstSample := groupSamples[0]
		row := firstSample["row"].([]string)

		labels := map[string]string{
			"__name__": metric.Name,
		}

		// Add dynamic labels
		for _, label := range metric.Labels {
			if label.StaticValue != "" {
				labels[label.LabelName] = label.StaticValue
			} else if label.IndexInCSVData < len(row) {
				labels[label.LabelName] = row[label.IndexInCSVData]
			}
		}

		// Add configured labels
		for labelKey, labelValue := range g.labels {
			labels[labelKey] = labelValue
		}

		// Create samples array for this time series
		var timeSeriesSamples []map[string]interface{}
		for _, sample := range groupSamples {
			timeSeriesSamples = append(timeSeriesSamples, map[string]interface{}{
				"value":     sample["value"],
				"timestamp": sample["timestamp"],
			})
		}

		// Create time series
		timeSeries := map[string]interface{}{
			"labels":  []map[string]string{labels},
			"samples": timeSeriesSamples,
		}

		samples = append(samples, timeSeries)
	}

	return samples
}

// parseFloat parses a string to float64 for GEM stream
func (g *GEMStream) parseFloat(s string) (float64, bool) {
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f, true
	}
	return 0, false
}

// parseInt64 parses a string to int64 for GEM stream
func (g *GEMStream) parseInt64(s string) (int64, bool) {
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return i, true
	}
	return 0, false
}

// toFloat64 converts a value to float64 if possible
func (g *GEMStream) toFloat64(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	default:
		return 0, false
	}
}

// Close closes the GEM stream
func (g *GEMStream) Close() error {
	return nil
}

// GetType returns the stream type
func (g *GEMStream) GetType() string {
	return "gem"
}

// OTELStream handles loading to OpenTelemetry collector
type OTELStream struct {
	endpoint   string
	httpClient *http.Client
	labels     map[string]string
}

// NewOTELStream creates a new OTEL stream
func NewOTELStream(config map[string]interface{}, labels map[string]string, insecureTLS bool, metrics []config.PrometheusMetricConfig) (*OTELStream, error) {
	endpoint, ok := config["endpoint"].(string)
	if !ok {
		return nil, fmt.Errorf("otel stream requires 'endpoint' configuration")
	}

	timeout := 30 * time.Second
	if t, ok := config["timeout"].(string); ok {
		if parsed, err := time.ParseDuration(t); err == nil {
			timeout = parsed
		}
	}

	// Configure HTTP client with TLS settings
	transport := &http.Transport{}
	if insecureTLS {
		transport.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	}

	return &OTELStream{
		endpoint: endpoint,
		labels:   labels,
		httpClient: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
	}, nil
}

// Load loads data to OTEL collector
func (o *OTELStream) Load(ctx context.Context, results []*transform.TransformedResult) error {
	// Convert results to OTEL format
	otelData := o.convertToOTELFormat(results)

	jsonData, err := json.Marshal(otelData)
	if err != nil {
		return fmt.Errorf("failed to marshal OTEL data: %w", err)
	}

	// Send to OTEL collector
	req, err := http.NewRequestWithContext(ctx, "POST", o.endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("OTEL collector returned status %d", resp.StatusCode)
	}

	return nil
}

// convertToOTELFormat converts results to OTEL format
func (o *OTELStream) convertToOTELFormat(results []*transform.TransformedResult) map[string]interface{} {
	var metrics []map[string]interface{}

	for _, result := range results {
		// Create attributes map with source
		attributes := map[string]interface{}{
			"source": result.Source,
		}

		// Add cluster name from metadata if available
		if clusterName, ok := result.Metadata["cluster_name"].(string); ok && clusterName != "" {
			attributes["cluster"] = clusterName
		}

		// Add configured labels as attributes
		for labelKey, labelValue := range o.labels {
			attributes[labelKey] = labelValue
		}

		metric := map[string]interface{}{
			"name":        "elasticetl_metric",
			"description": "Metric from ElasticETL",
			"unit":        "1",
			"data": map[string]interface{}{
				"dataPoints": []map[string]interface{}{
					{
						"attributes":   attributes,
						"timeUnixNano": result.Timestamp.UnixNano(),
						"value":        result.TransformedData,
					},
				},
			},
		}
		metrics = append(metrics, metric)
	}

	return map[string]interface{}{
		"resourceMetrics": []map[string]interface{}{
			{
				"resource": map[string]interface{}{
					"attributes": []map[string]interface{}{
						{
							"key":   "service.name",
							"value": map[string]string{"stringValue": "elasticetl"},
						},
					},
				},
				"scopeMetrics": []map[string]interface{}{
					{
						"scope": map[string]interface{}{
							"name":    "elasticetl",
							"version": "1.0.0",
						},
						"metrics": metrics,
					},
				},
			},
		},
	}
}

// Close closes the OTEL stream
func (o *OTELStream) Close() error {
	return nil
}

// GetType returns the stream type
func (o *OTELStream) GetType() string {
	return "otel"
}

// DynamicLabelConfig defines how to create labels from CSV data
type DynamicLabelConfig struct {
	LabelName   string `json:"label_name" yaml:"label_name"`
	CSVColumn   string `json:"csv_column,omitempty" yaml:"csv_column,omitempty"`
	StaticValue string `json:"static_value,omitempty" yaml:"static_value,omitempty"`
}

// MetricColumnConfig defines which CSV columns to use as metrics
type MetricColumnConfig struct {
	Column     string `json:"column" yaml:"column"`
	MetricName string `json:"metric_name" yaml:"metric_name"`
}

// PrometheusStream handles loading to Prometheus
type PrometheusStream struct {
	endpoint      string
	httpClient    *http.Client
	labels        map[string]string
	dynamicLabels []DynamicLabelConfig
	metricColumns []MetricColumnConfig
	basicAuth     string
}

// NewPrometheusStream creates a new Prometheus stream
func NewPrometheusStream(config map[string]interface{}, labels map[string]string, insecureTLS bool, metrics []config.PrometheusMetricConfig) (*PrometheusStream, error) {
	// Support both old endpoint format and new remote_write_url format
	var endpoint string
	if ep, ok := config["endpoint"].(string); ok {
		endpoint = ep
	} else if rwUrl, ok := config["remote_write_url"].(string); ok {
		endpoint = rwUrl
	} else {
		return nil, fmt.Errorf("prometheus stream requires 'endpoint' or 'remote_write_url' configuration")
	}

	timeout := 30 * time.Second
	if t, ok := config["timeout"].(string); ok {
		if parsed, err := time.ParseDuration(t); err == nil {
			timeout = parsed
		}
	}

	// Configure HTTP client with TLS settings
	transport := &http.Transport{}
	if insecureTLS {
		transport.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	}

	stream := &PrometheusStream{
		endpoint: endpoint,
		labels:   labels,
		httpClient: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
	}

	// Parse dynamic labels configuration
	if dynamicLabelsRaw, ok := config["dynamic_labels"]; ok {
		if dynamicLabelsSlice, ok := dynamicLabelsRaw.([]interface{}); ok {
			for _, labelRaw := range dynamicLabelsSlice {
				if labelMap, ok := labelRaw.(map[string]interface{}); ok {
					var labelConfig DynamicLabelConfig
					if labelName, ok := labelMap["label_name"].(string); ok {
						labelConfig.LabelName = labelName
					}
					if csvColumn, ok := labelMap["csv_column"].(string); ok {
						labelConfig.CSVColumn = csvColumn
					}
					if staticValue, ok := labelMap["static_value"].(string); ok {
						labelConfig.StaticValue = staticValue
					}
					stream.dynamicLabels = append(stream.dynamicLabels, labelConfig)
				}
			}
		}
	}

	// Parse metric columns configuration
	if metricColumnsRaw, ok := config["metric_columns"]; ok {
		if metricColumnsSlice, ok := metricColumnsRaw.([]interface{}); ok {
			for _, metricRaw := range metricColumnsSlice {
				if metricMap, ok := metricRaw.(map[string]interface{}); ok {
					var metricConfig MetricColumnConfig
					if column, ok := metricMap["column"].(string); ok {
						metricConfig.Column = column
					}
					if metricName, ok := metricMap["metric_name"].(string); ok {
						metricConfig.MetricName = metricName
					}
					stream.metricColumns = append(stream.metricColumns, metricConfig)
				}
			}
		}
	}

	// Parse basic auth configuration
	basicAuth, err := parseBasicAuth(config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse basic auth: %w", err)
	}
	stream.basicAuth = basicAuth

	return stream, nil
}

// Load loads data to Prometheus
func (p *PrometheusStream) Load(ctx context.Context, results []*transform.TransformedResult) error {
	// Convert to Prometheus exposition format
	metricsText := p.convertToPrometheusFormat(results)

	// Send to Prometheus pushgateway
	req, err := http.NewRequestWithContext(ctx, "POST", p.endpoint, bytes.NewBufferString(metricsText))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "text/plain")

	// Add basic auth header if configured
	if p.basicAuth != "" {
		req.Header.Set("Authorization", p.basicAuth)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("Prometheus returned status %d", resp.StatusCode)
	}

	return nil
}

// convertToPrometheusFormat converts results to Prometheus exposition format
func (p *PrometheusStream) convertToPrometheusFormat(results []*transform.TransformedResult) string {
	var lines []string

	for _, result := range results {
		for key, value := range result.TransformedData {
			if numValue, ok := p.toFloat64(value); ok {
				// Build labels string
				labelPairs := []string{fmt.Sprintf(`source="%s"`, result.Source)}

				// Add cluster name from metadata if available
				if clusterName, ok := result.Metadata["cluster_name"].(string); ok && clusterName != "" {
					labelPairs = append(labelPairs, fmt.Sprintf(`cluster="%s"`, clusterName))
				}

				// Add configured labels
				for labelKey, labelValue := range p.labels {
					labelPairs = append(labelPairs, fmt.Sprintf(`%s="%s"`, labelKey, labelValue))
				}

				labelsStr := fmt.Sprintf("{%s}", fmt.Sprintf("%s", labelPairs))
				line := fmt.Sprintf(`%s%s %f %d`,
					key, labelsStr, numValue, result.Timestamp.UnixMilli())
				lines = append(lines, line)
			}
		}
	}

	return fmt.Sprintf("%s\n", fmt.Sprintf("%s", lines))
}

// toFloat64 converts a value to float64 if possible
func (p *PrometheusStream) toFloat64(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	default:
		return 0, false
	}
}

// Close closes the Prometheus stream
func (p *PrometheusStream) Close() error {
	return nil
}

// GetType returns the stream type
func (p *PrometheusStream) GetType() string {
	return "prometheus"
}

// DebugStream handles loading to debug files
type DebugStream struct {
	path   string
	format string // "json", "prometheus", "otel"
}

// NewDebugStream creates a new debug stream
func NewDebugStream(config map[string]interface{}, metrics []config.PrometheusMetricConfig) (*DebugStream, error) {
	path, ok := config["path"].(string)
	if !ok {
		return nil, fmt.Errorf("debug stream requires 'path' configuration")
	}

	format := "json" // default format
	if f, ok := config["format"].(string); ok {
		format = f
	}

	return &DebugStream{
		path:   path,
		format: format,
	}, nil
}

// Load loads data to debug file
func (d *DebugStream) Load(ctx context.Context, results []*transform.TransformedResult) error {
	// Create debug directory if it doesn't exist
	debugDir := filepath.Dir(d.path)
	if err := os.MkdirAll(debugDir, 0755); err != nil {
		return fmt.Errorf("failed to create debug directory: %w", err)
	}

	var outputData []byte
	var fileExtension string
	var err error

	switch d.format {
	case "prometheus":
		// Generate Prometheus timeseries format
		outputData, fileExtension, err = d.generatePrometheusFormat(results)
	case "otel":
		// Generate OTEL format
		outputData, fileExtension, err = d.generateOTELFormat(results)
	default:
		// Default JSON format
		outputData, fileExtension, err = d.generateJSONFormat(results)
	}

	if err != nil {
		return fmt.Errorf("failed to generate debug output: %w", err)
	}

	// Generate filename with timestamp
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("%s_load_%s.%s", filepath.Base(d.path), timestamp, fileExtension)
	fullPath := filepath.Join(debugDir, filename)

	// Write to file
	if err := os.WriteFile(fullPath, outputData, 0644); err != nil {
		return fmt.Errorf("failed to write debug file: %w", err)
	}

	fmt.Printf("Debug load output (%s format) written to: %s\n", d.format, fullPath)
	return nil
}

// generateJSONFormat generates the default JSON debug format
func (d *DebugStream) generateJSONFormat(results []*transform.TransformedResult) ([]byte, string, error) {
	debugData := map[string]interface{}{
		"timestamp":     time.Now().Format(time.RFC3339),
		"pipeline":      "load",
		"format":        "json",
		"results_count": len(results),
		"results":       results,
	}

	jsonData, err := json.MarshalIndent(debugData, "", "  ")
	return jsonData, "json", err
}

// generatePrometheusFormat generates Prometheus timeseries format using CSV data
func (d *DebugStream) generatePrometheusFormat(results []*transform.TransformedResult) ([]byte, string, error) {
	var lines []string

	// Add header comment
	lines = append(lines, fmt.Sprintf("# ElasticETL Debug Output - Prometheus Format"))
	lines = append(lines, fmt.Sprintf("# Generated at: %s", time.Now().Format(time.RFC3339)))
	lines = append(lines, fmt.Sprintf("# Results count: %d", len(results)))
	lines = append(lines, "")

	for _, result := range results {
		// Use CSV data to create time series
		if len(result.CSVData) == 0 {
			continue
		}

		// Parse metrics configuration from result metadata if available
		metricsConfig := d.parseMetricsConfig(result)
		if len(metricsConfig) == 0 {
			// Fallback to old behavior if no metrics config
			d.generateFallbackPrometheusFormat(result, &lines)
			continue
		}

		// Generate time series for each metric
		for _, metric := range metricsConfig {
			timeSeries := d.createTimeSeriesForMetric(result.CSVData, metric)
			for _, ts := range timeSeries {
				lines = append(lines, ts)
			}
		}
	}

	output := strings.Join(lines, "\n") + "\n"
	return []byte(output), "txt", nil
}

// parseMetricsConfig extracts metrics configuration from result metadata
func (d *DebugStream) parseMetricsConfig(result *transform.TransformedResult) []config.PrometheusMetricConfig {
	var metrics []config.PrometheusMetricConfig

	// Try to get metrics config from metadata
	if metricsRaw, ok := result.Metadata["metrics"]; ok {
		if metricsList, ok := metricsRaw.([]interface{}); ok {
			for _, metricRaw := range metricsList {
				if metricMap, ok := metricRaw.(map[string]interface{}); ok {
					var metric config.PrometheusMetricConfig

					if name, ok := metricMap["name"].(string); ok {
						metric.Name = name
					}

					if value, ok := metricMap["value"].(int); ok {
						metric.Value = value
					}

					if timestamp, ok := metricMap["timestamp"].(int); ok {
						metric.Timestamp = timestamp
					}

					if uniqueFields, ok := metricMap["uniquefieldsIndex"].([]interface{}); ok {
						for _, field := range uniqueFields {
							if idx, ok := field.(int); ok {
								metric.UniqueFieldsIndex = append(metric.UniqueFieldsIndex, idx)
							}
						}
					}

					if labelsRaw, ok := metricMap["labels"].([]interface{}); ok {
						for _, labelRaw := range labelsRaw {
							if labelMap, ok := labelRaw.(map[string]interface{}); ok {
								var label config.PrometheusLabelConfig

								if labelName, ok := labelMap["label_name"].(string); ok {
									label.LabelName = labelName
								}

								if indexInCSV, ok := labelMap["index_in_csv_data"].(int); ok {
									label.IndexInCSVData = indexInCSV
								}

								if staticValue, ok := labelMap["static_value"].(string); ok {
									label.StaticValue = staticValue
								}

								metric.Labels = append(metric.Labels, label)
							}
						}
					}

					metrics = append(metrics, metric)
				}
			}
		}
	}

	return metrics
}

// createTimeSeriesForMetric creates time series for a specific metric
func (d *DebugStream) createTimeSeriesForMetric(csvData [][]string, metric config.PrometheusMetricConfig) []string {
	var lines []string

	// Group CSV rows by unique field combinations
	uniqueGroups := make(map[string][]map[string]interface{})

	for _, row := range csvData {
		if len(row) <= metric.Value || len(row) <= metric.Timestamp {
			continue // Skip rows that don't have required columns
		}

		// Create unique key from uniqueFieldsIndex
		var keyParts []string
		for _, idx := range metric.UniqueFieldsIndex {
			if idx < len(row) {
				keyParts = append(keyParts, row[idx])
			}
		}
		uniqueKey := strings.Join(keyParts, "|")

		// Parse value and timestamp
		value, ok := d.parseFloat(row[metric.Value])
		if !ok {
			continue
		}

		timestamp, ok := d.parseInt64(row[metric.Timestamp])
		if !ok {
			continue
		}

		// Create sample
		sample := map[string]interface{}{
			"value":     value,
			"timestamp": timestamp,
			"row":       row,
		}

		uniqueGroups[uniqueKey] = append(uniqueGroups[uniqueKey], sample)
	}

	// Generate time series for each unique group
	for _, samples := range uniqueGroups {
		if len(samples) == 0 {
			continue
		}

		// Build labels from first sample in group
		firstSample := samples[0]
		row := firstSample["row"].([]string)

		var labelPairs []string
		labelPairs = append(labelPairs, fmt.Sprintf(`__name__="%s"`, metric.Name))

		// Add dynamic labels
		for _, label := range metric.Labels {
			if label.StaticValue != "" {
				labelPairs = append(labelPairs, fmt.Sprintf(`%s="%s"`, label.LabelName, label.StaticValue))
			} else if label.IndexInCSVData < len(row) {
				labelPairs = append(labelPairs, fmt.Sprintf(`%s="%s"`, label.LabelName, row[label.IndexInCSVData]))
			}
		}

		labelsStr := strings.Join(labelPairs, ", ")

		// Generate timeseries block
		lines = append(lines, fmt.Sprintf("timeseries {"))
		lines = append(lines, fmt.Sprintf("  labels: { %s }", labelsStr))

		// Add all samples for this unique group
		for _, sample := range samples {
			value := sample["value"].(float64)
			timestamp := sample["timestamp"].(int64)
			lines = append(lines, fmt.Sprintf("  samples: { timestamp: %d, value: %g }", timestamp, value))
		}

		lines = append(lines, "}")
		lines = append(lines, "")
	}

	return lines
}

// generateFallbackPrometheusFormat generates fallback format when no metrics config is available
func (d *DebugStream) generateFallbackPrometheusFormat(result *transform.TransformedResult, lines *[]string) {
	for key, value := range result.TransformedData {
		if numValue, ok := d.toFloat64(value); ok {
			// Build labels string
			labelPairs := []string{fmt.Sprintf(`source="%s"`, result.Source)}

			// Add cluster name from metadata if available
			if clusterName, ok := result.Metadata["cluster_name"].(string); ok && clusterName != "" {
				labelPairs = append(labelPairs, fmt.Sprintf(`cluster="%s"`, clusterName))
			}

			labelsStr := strings.Join(labelPairs, ",")
			line := fmt.Sprintf(`%s{%s} %f %d`,
				key, labelsStr, numValue, result.Timestamp.UnixMilli())
			*lines = append(*lines, line)
		}
	}
}

// parseFloat parses a string to float64
func (d *DebugStream) parseFloat(s string) (float64, bool) {
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f, true
	}
	return 0, false
}

// parseInt64 parses a string to int64
func (d *DebugStream) parseInt64(s string) (int64, bool) {
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return i, true
	}
	return 0, false
}

// generateOTELFormat generates OTEL collector format
func (d *DebugStream) generateOTELFormat(results []*transform.TransformedResult) ([]byte, string, error) {
	var metrics []map[string]interface{}

	for _, result := range results {
		// Create attributes map with source
		attributes := map[string]interface{}{
			"source": result.Source,
		}

		// Add cluster name from metadata if available
		if clusterName, ok := result.Metadata["cluster_name"].(string); ok && clusterName != "" {
			attributes["cluster"] = clusterName
		}

		metric := map[string]interface{}{
			"name":        "elasticetl_metric",
			"description": "Metric from ElasticETL",
			"unit":        "1",
			"data": map[string]interface{}{
				"dataPoints": []map[string]interface{}{
					{
						"attributes":   attributes,
						"timeUnixNano": result.Timestamp.UnixNano(),
						"value":        result.TransformedData,
					},
				},
			},
		}
		metrics = append(metrics, metric)
	}

	otelData := map[string]interface{}{
		"timestamp": time.Now().Format(time.RFC3339),
		"pipeline":  "load",
		"format":    "otel",
		"resourceMetrics": []map[string]interface{}{
			{
				"resource": map[string]interface{}{
					"attributes": []map[string]interface{}{
						{
							"key":   "service.name",
							"value": map[string]string{"stringValue": "elasticetl"},
						},
					},
				},
				"scopeMetrics": []map[string]interface{}{
					{
						"scope": map[string]interface{}{
							"name":    "elasticetl",
							"version": "1.0.0",
						},
						"metrics": metrics,
					},
				},
			},
		},
	}

	jsonData, err := json.MarshalIndent(otelData, "", "  ")
	return jsonData, "json", err
}

// toFloat64 converts a value to float64 if possible
func (d *DebugStream) toFloat64(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	default:
		return 0, false
	}
}

// Close closes the debug stream
func (d *DebugStream) Close() error {
	return nil
}

// GetType returns the stream type
func (d *DebugStream) GetType() string {
	return "debug"
}

// CSVStream handles loading to CSV files
type CSVStream struct {
	path string
}

// NewCSVStream creates a new CSV stream
func NewCSVStream(config map[string]interface{}) (*CSVStream, error) {
	path, ok := config["path"].(string)
	if !ok {
		return nil, fmt.Errorf("csv stream requires 'path' configuration")
	}

	return &CSVStream{
		path: path,
	}, nil
}

// Load loads data to CSV file
func (c *CSVStream) Load(ctx context.Context, results []*transform.TransformedResult) error {
	if len(results) == 0 {
		return nil
	}

	// Create CSV directory if it doesn't exist
	csvDir := filepath.Dir(c.path)
	if err := os.MkdirAll(csvDir, 0755); err != nil {
		return fmt.Errorf("failed to create CSV directory: %w", err)
	}

	// Generate filename with timestamp
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("%s_%s.csv", filepath.Base(c.path), timestamp)
	fullPath := filepath.Join(csvDir, filename)

	// Create CSV file
	file, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("failed to create CSV file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write CSV data from transformed results
	for _, result := range results {
		if len(result.CSVHeaders) > 0 && len(result.CSVData) > 0 {
			// Write headers (only for first result)
			if result == results[0] {
				if err := writer.Write(result.CSVHeaders); err != nil {
					return fmt.Errorf("failed to write CSV headers: %w", err)
				}
			}

			// Write data rows
			for _, row := range result.CSVData {
				if err := writer.Write(row); err != nil {
					return fmt.Errorf("failed to write CSV row: %w", err)
				}
			}
		}
	}

	fmt.Printf("CSV output written to: %s\n", fullPath)
	return nil
}

// Close closes the CSV stream
func (c *CSVStream) Close() error {
	return nil
}

// GetType returns the stream type
func (c *CSVStream) GetType() string {
	return "csv"
}
