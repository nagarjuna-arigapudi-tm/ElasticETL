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

	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
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

	// Use safe map conversion to handle both JSON and YAML parsing
	basicAuthMap, ok := safeMapStringInterface(basicAuthRaw)
	if !ok {
		return "", fmt.Errorf("basic_auth must be an object")
	}

	// Use safe string conversion to handle both JSON and YAML parsing
	username, ok := safeString(basicAuthMap["username"])
	if !ok {
		return "", fmt.Errorf("basic_auth.username is required")
	}

	password, ok := safeString(basicAuthMap["password"])
	if !ok {
		return "", fmt.Errorf("basic_auth.password is required")
	}

	return createBasicAuthHeader(username, password), nil
}

// safeString safely converts a value to string, handling both JSON and YAML parsing
func safeString(value interface{}) (string, bool) {
	if value == nil {
		return "", false
	}

	switch v := value.(type) {
	case string:
		return v, true
	case int:
		return fmt.Sprintf("%d", v), true
	case int64:
		return fmt.Sprintf("%d", v), true
	case float64:
		// Check if it's actually an integer value
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d", int64(v)), true
		}
		return fmt.Sprintf("%g", v), true
	case float32:
		if v == float32(int32(v)) {
			return fmt.Sprintf("%d", int32(v)), true
		}
		return fmt.Sprintf("%g", v), true
	case bool:
		return fmt.Sprintf("%t", v), true
	default:
		return fmt.Sprintf("%v", v), true
	}
}

// safeMapStringInterface safely converts a value to map[string]interface{}, handling both JSON and YAML parsing
func safeMapStringInterface(value interface{}) (map[string]interface{}, bool) {
	if value == nil {
		return nil, false
	}

	switch v := value.(type) {
	case map[string]interface{}:
		return v, true
	case map[interface{}]interface{}:
		// YAML sometimes parses maps as map[interface{}]interface{}
		result := make(map[string]interface{})
		for key, val := range v {
			if strKey, ok := safeString(key); ok {
				result[strKey] = val
			}
		}
		return result, true
	default:
		return nil, false
	}
}

// hasCSVData checks if any of the transformed results contain CSV data
func hasCSVData(results []*transform.TransformedResult) bool {
	for _, result := range results {
		if len(result.CSVData) > 0 {
			return true
		}
	}
	return false
}

// Loader handles data loading to various destinations
type Loader struct {
	config        config.LoadConfig
	streams       []Stream
	streamConfigs []config.StreamConfig // Store original stream configs for debug access
	mutex         sync.RWMutex
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
		config:        cfg,
		streamConfigs: cfg.Streams, // Store original stream configs for debug access
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
func (l *Loader) Load(ctx context.Context, results []*transform.TransformedResult, pipelineName string) error {
	l.mutex.RLock()
	streams := make([]Stream, len(l.streams))
	copy(streams, l.streams)
	streamConfigs := make([]config.StreamConfig, len(l.streamConfigs))
	copy(streamConfigs, l.streamConfigs)
	globalDebugConfig := l.config.Debug
	l.mutex.RUnlock()

	var wg sync.WaitGroup
	errorsChan := make(chan error, len(streams))

	// Load to all streams concurrently
	for i, stream := range streams {
		wg.Add(1)
		go func(s Stream, streamCfg config.StreamConfig) {
			defer wg.Done()

			// Use per-stream debug config if available, otherwise fall back to global
			var debugConfig config.LoadDebugConfig
			if streamCfg.Debug != nil {
				debugConfig = *streamCfg.Debug
			} else {
				debugConfig = globalDebugConfig
			}

			// Add stream name to debug output if available
			streamName := streamCfg.Name
			if streamName == "" {
				streamName = s.GetType()
			}

			if err := l.loadWithStreamDebug(ctx, results, s, pipelineName, streamName, debugConfig); err != nil {
				errorsChan <- fmt.Errorf("stream %s: %w", s.GetType(), err)
			}
		}(stream, streamConfigs[i])
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
	l.streamConfigs = cfg.Streams // Update stream configs for debug access
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
	case "prometheus_remote_write":
		return NewPrometheusRemoteWriteStream(cfg.Config, cfg.Labels, cfg.InsecureTLS, metrics)
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
	endpoint, ok := safeString(config["endpoint"])
	if !ok {
		return nil, fmt.Errorf("gem stream requires 'endpoint' configuration")
	}

	timeout := 30 * time.Second
	if t, ok := safeString(config["timeout"]); ok {
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
	// Check if there's any CSV data to load
	if !hasCSVData(results) {
		return nil // Skip API call when no data to load
	}

	// Convert results to Prometheus remote write format
	timeSeries := g.convertToPrometheusTimeSeries(results)
	if len(timeSeries) == 0 {
		return nil
	}

	// Create WriteRequest
	writeRequest := &prompb.WriteRequest{}
	for _, ts := range timeSeries {
		writeRequest.Timeseries = append(writeRequest.Timeseries, *ts)
	}

	// Marshal to protobuf
	data, err := writeRequest.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal write request: %w", err)
	}

	// Compress with snappy
	compressed := snappy.Encode(nil, data)

	// Send to GEM endpoint
	req, err := http.NewRequestWithContext(ctx, "POST", g.endpoint, bytes.NewReader(compressed))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-protobuf")
	req.Header.Set("Content-Encoding", "snappy")
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

// convertToPrometheusTimeSeries converts transformed results to Prometheus time series using CSV data
func (g *GEMStream) convertToPrometheusTimeSeries(results []*transform.TransformedResult) []*prompb.TimeSeries {
	var timeSeries []*prompb.TimeSeries

	for _, result := range results {
		// Use CSV data to create time series if available and metrics are configured
		if len(result.CSVData) > 0 && len(g.metrics) > 0 {
			// Generate time series for each metric using CSV data
			for _, metric := range g.metrics {
				metricTimeSeries := g.createTimeSeriesForMetric(result.CSVData, metric)
				timeSeries = append(timeSeries, metricTimeSeries...)
			}
			continue
		}

		// Fallback to old behavior using TransformedData
		timestamp := result.Timestamp.UnixMilli()
		for key, value := range result.TransformedData {
			// Only include numeric values as metrics
			if numValue, ok := g.toFloat64(value); ok {
				// Create labels
				var labels []prompb.Label
				labels = append(labels, prompb.Label{Name: "__name__", Value: key})
				labels = append(labels, prompb.Label{Name: "source", Value: result.Source})

				// Add cluster name from metadata if available
				if clusterName, ok := safeString(result.Metadata["cluster_name"]); ok && clusterName != "" {
					labels = append(labels, prompb.Label{Name: "cluster", Value: clusterName})
				}

				// Add configured labels
				for labelKey, labelValue := range g.labels {
					labels = append(labels, prompb.Label{Name: labelKey, Value: labelValue})
				}

				// Create time series
				ts := &prompb.TimeSeries{
					Labels: labels,
					Samples: []prompb.Sample{
						{
							Value:     numValue,
							Timestamp: timestamp,
						},
					},
				}
				timeSeries = append(timeSeries, ts)
			}
		}
	}

	return timeSeries
}

// createTimeSeriesForMetric creates Prometheus remote write time series for a specific metric using CSV data
func (g *GEMStream) createTimeSeriesForMetric(csvData [][]string, metric config.PrometheusMetricConfig) []*prompb.TimeSeries {
	var timeSeries []*prompb.TimeSeries

	// Group CSV rows by unique field combinations
	uniqueGroups := make(map[string][]map[string]interface{})

	for _, row := range csvData {
		// Check bounds for required columns
		if metric.Value >= len(row) || metric.Timestamp >= len(row) {
			continue // Skip rows that don't have required columns
		}

		// Create unique key from uniqueFieldsIndex with bounds checking
		var keyParts []string
		for _, idx := range metric.UniqueFieldsIndex {
			if idx >= 0 && idx < len(row) {
				keyParts = append(keyParts, row[idx])
			}
		}
		uniqueKey := strings.Join(keyParts, "|")

		// Parse value and timestamp with bounds checking
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

		var labels []prompb.Label
		labels = append(labels, prompb.Label{Name: "__name__", Value: metric.Name})

		// Add dynamic labels with bounds checking
		for _, label := range metric.Labels {
			if label.StaticValue != "" {
				labels = append(labels, prompb.Label{Name: label.LabelName, Value: label.StaticValue})
			} else if label.IndexInCSVData >= 0 && label.IndexInCSVData < len(row) {
				labels = append(labels, prompb.Label{Name: label.LabelName, Value: row[label.IndexInCSVData]})
			}
		}

		// Add configured labels
		for labelKey, labelValue := range g.labels {
			labels = append(labels, prompb.Label{Name: labelKey, Value: labelValue})
		}

		// Create samples array for this time series
		var samples []prompb.Sample
		for _, sample := range groupSamples {
			samples = append(samples, prompb.Sample{
				Value:     sample["value"].(float64),
				Timestamp: sample["timestamp"].(int64),
			})
		}

		// Create time series
		ts := &prompb.TimeSeries{
			Labels:  labels,
			Samples: samples,
		}

		timeSeries = append(timeSeries, ts)
	}

	return timeSeries
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
				if clusterName, ok := safeString(result.Metadata["cluster_name"]); ok && clusterName != "" {
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
		// Check bounds for required columns
		if metric.Value >= len(row) || metric.Timestamp >= len(row) {
			continue // Skip rows that don't have required columns
		}

		// Create unique key from uniqueFieldsIndex with bounds checking
		var keyParts []string
		for _, idx := range metric.UniqueFieldsIndex {
			if idx >= 0 && idx < len(row) {
				keyParts = append(keyParts, row[idx])
			}
		}
		uniqueKey := strings.Join(keyParts, "|")

		// Parse value and timestamp with bounds checking
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

		// Add dynamic labels with bounds checking
		for _, label := range metric.Labels {
			if label.StaticValue != "" {
				labels[label.LabelName] = label.StaticValue
			} else if label.IndexInCSVData >= 0 && label.IndexInCSVData < len(row) {
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
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return int64(f), true
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
	endpoint, ok := safeString(config["endpoint"])
	if !ok {
		return nil, fmt.Errorf("otel stream requires 'endpoint' configuration")
	}

	timeout := 30 * time.Second
	if t, ok := safeString(config["timeout"]); ok {
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
	// Check if there's any CSV data to load
	if !hasCSVData(results) {
		return nil // Skip API call when no data to load
	}

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
		if clusterName, ok := safeString(result.Metadata["cluster_name"]); ok && clusterName != "" {
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
	if ep, ok := safeString(config["endpoint"]); ok {
		endpoint = ep
	} else if rwUrl, ok := safeString(config["remote_write_url"]); ok {
		endpoint = rwUrl
	} else {
		return nil, fmt.Errorf("prometheus stream requires 'endpoint' or 'remote_write_url' configuration")
	}

	timeout := 30 * time.Second
	if t, ok := safeString(config["timeout"]); ok {
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
				if labelMap, ok := safeMapStringInterface(labelRaw); ok {
					var labelConfig DynamicLabelConfig
					if labelName, ok := safeString(labelMap["label_name"]); ok {
						labelConfig.LabelName = labelName
					}
					if csvColumn, ok := safeString(labelMap["csv_column"]); ok {
						labelConfig.CSVColumn = csvColumn
					}
					if staticValue, ok := safeString(labelMap["static_value"]); ok {
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
				if metricMap, ok := safeMapStringInterface(metricRaw); ok {
					var metricConfig MetricColumnConfig
					if column, ok := safeString(metricMap["column"]); ok {
						metricConfig.Column = column
					}
					if metricName, ok := safeString(metricMap["metric_name"]); ok {
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
	// Check if there's any CSV data to load
	if !hasCSVData(results) {
		return nil // Skip API call when no data to load
	}

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
		// Use CSV data to create time series if available and metric columns are configured
		if len(result.CSVData) > 0 && len(p.metricColumns) > 0 {
			// Generate time series using CSV data and metric columns configuration
			prometheusLines := p.createPrometheusLinesFromCSV(result.CSVData, result.CSVHeaders, result.Source, result.Timestamp.UnixMilli())
			lines = append(lines, prometheusLines...)
			continue
		}

		// Fallback to old behavior using TransformedData
		for key, value := range result.TransformedData {
			if numValue, ok := p.toFloat64(value); ok {
				// Build labels string
				labelPairs := []string{fmt.Sprintf(`source="%s"`, result.Source)}

				// Add cluster name from metadata if available
				if clusterName, ok := safeString(result.Metadata["cluster_name"]); ok && clusterName != "" {
					labelPairs = append(labelPairs, fmt.Sprintf(`cluster="%s"`, clusterName))
				}

				// Add configured labels
				for labelKey, labelValue := range p.labels {
					labelPairs = append(labelPairs, fmt.Sprintf(`%s="%s"`, labelKey, labelValue))
				}

				labelsStr := strings.Join(labelPairs, ",")
				line := fmt.Sprintf(`%s{%s} %f %d`,
					key, labelsStr, numValue, result.Timestamp.UnixMilli())
				lines = append(lines, line)
			}
		}
	}

	return strings.Join(lines, "\n") + "\n"
}

// createPrometheusLinesFromCSV creates Prometheus exposition format lines from CSV data
func (p *PrometheusStream) createPrometheusLinesFromCSV(csvData [][]string, csvHeaders []string, source string, timestamp int64) []string {
	var lines []string

	// Create a map of header names to column indices for easier lookup
	headerMap := make(map[string]int)
	for i, header := range csvHeaders {
		headerMap[header] = i
	}

	// Process each configured metric column
	for _, metricConfig := range p.metricColumns {
		// Find the column index for this metric
		columnIndex, exists := headerMap[metricConfig.Column]
		if !exists {
			continue // Skip if column doesn't exist
		}

		// Process each row of CSV data
		for _, row := range csvData {
			if columnIndex >= len(row) {
				continue // Skip if row doesn't have this column
			}

			// Parse the metric value
			if numValue, ok := p.parseFloat(row[columnIndex]); ok {
				// Build labels string
				labelPairs := []string{fmt.Sprintf(`source="%s"`, source)}

				// Add dynamic labels from CSV columns
				for _, labelConfig := range p.dynamicLabels {
					if labelConfig.StaticValue != "" {
						labelPairs = append(labelPairs, fmt.Sprintf(`%s="%s"`, labelConfig.LabelName, labelConfig.StaticValue))
					} else if labelConfig.CSVColumn != "" {
						if labelColumnIndex, exists := headerMap[labelConfig.CSVColumn]; exists && labelColumnIndex < len(row) {
							labelPairs = append(labelPairs, fmt.Sprintf(`%s="%s"`, labelConfig.LabelName, row[labelColumnIndex]))
						}
					}
				}

				// Add configured static labels
				for labelKey, labelValue := range p.labels {
					labelPairs = append(labelPairs, fmt.Sprintf(`%s="%s"`, labelKey, labelValue))
				}

				labelsStr := strings.Join(labelPairs, ",")
				line := fmt.Sprintf(`%s{%s} %f %d`,
					metricConfig.MetricName, labelsStr, numValue, timestamp)
				lines = append(lines, line)
			}
		}
	}

	return lines
}

// parseFloat parses a string to float64 for Prometheus stream
func (p *PrometheusStream) parseFloat(s string) (float64, bool) {
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f, true
	}
	return 0, false
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
	path         string
	format       string // "json", "prometheus", "otel"
	metrics      []config.PrometheusMetricConfig
	pipelineName string // Store pipeline name for filename generation
}

// NewDebugStream creates a new debug stream
func NewDebugStream(config map[string]interface{}, metrics []config.PrometheusMetricConfig) (*DebugStream, error) {
	path, ok := safeString(config["path"])
	if !ok {
		return nil, fmt.Errorf("debug stream requires 'path' configuration")
	}

	format := "json" // default format
	if f, ok := safeString(config["format"]); ok {
		format = f
	}

	return &DebugStream{
		path:    path,
		format:  format,
		metrics: metrics,
	}, nil
}

// SetPipelineName sets the pipeline name for filename generation
func (d *DebugStream) SetPipelineName(pipelineName string) {
	d.pipelineName = pipelineName
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

	// Generate filename with timestamp and pipeline name
	timestamp := time.Now().Format("20060102_150405")
	pipelineName := d.pipelineName
	if pipelineName == "" {
		pipelineName = "unknown"
	}
	filename := fmt.Sprintf("%s_load_%s.%s", pipelineName, timestamp, fileExtension)
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

		// Get metrics configuration from loader config (passed during stream creation)
		if len(d.metrics) == 0 {
			// Fallback to old behavior if no metrics config
			d.generateFallbackPrometheusFormat(result, &lines)
			continue
		}

		// Generate time series for each metric using loader's metrics configuration
		for _, metric := range d.metrics {
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
		// Check bounds for required columns
		if metric.Value >= len(row) || metric.Timestamp >= len(row) {
			continue // Skip rows that don't have required columns
		}

		// Create unique key from uniqueFieldsIndex with bounds checking
		var keyParts []string
		for _, idx := range metric.UniqueFieldsIndex {
			if idx >= 0 && idx < len(row) {
				keyParts = append(keyParts, row[idx])
			}
		}
		uniqueKey := strings.Join(keyParts, "|")

		// Parse value and timestamp with bounds checking
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

		// Add dynamic labels with bounds checking
		for _, label := range metric.Labels {
			if label.StaticValue != "" {
				labelPairs = append(labelPairs, fmt.Sprintf(`%s="%s"`, label.LabelName, label.StaticValue))
			} else if label.IndexInCSVData >= 0 && label.IndexInCSVData < len(row) {
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
			if clusterName, ok := safeString(result.Metadata["cluster_name"]); ok && clusterName != "" {
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
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return int64(f), true
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
		if clusterName, ok := safeString(result.Metadata["cluster_name"]); ok && clusterName != "" {
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
	path, ok := safeString(config["path"])
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

// PrometheusRemoteWriteStream handles loading to Prometheus using remote write protocol
type PrometheusRemoteWriteStream struct {
	endpoint      string
	httpClient    *http.Client
	labels        map[string]string
	metrics       []config.PrometheusMetricConfig
	basicAuth     string
	clientLibrary string // "native" (default) or "m3dbx"
}

// NewPrometheusRemoteWriteStream creates a new Prometheus remote write stream
func NewPrometheusRemoteWriteStream(config map[string]interface{}, labels map[string]string, insecureTLS bool, metrics []config.PrometheusMetricConfig) (*PrometheusRemoteWriteStream, error) {
	endpoint, ok := safeString(config["remote_write_url"])
	if !ok {
		if ep, ok := safeString(config["endpoint"]); ok {
			endpoint = ep
		} else {
			return nil, fmt.Errorf("prometheus remote write stream requires 'remote_write_url' or 'endpoint' configuration")
		}
	}

	timeout := 30 * time.Second
	if t, ok := safeString(config["timeout"]); ok {
		if parsed, err := time.ParseDuration(t); err == nil {
			timeout = parsed
		}
	}

	// Parse client library configuration (default to "native")
	clientLibrary := "native"
	if cl, ok := safeString(config["client_library"]); ok {
		switch cl {
		case "native", "m3dbx":
			clientLibrary = cl
		default:
			return nil, fmt.Errorf("unsupported client_library '%s', supported values: 'native', 'm3dbx'", cl)
		}
	}

	// Configure HTTP client with TLS settings
	transport := &http.Transport{}
	if insecureTLS {
		transport.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	}

	stream := &PrometheusRemoteWriteStream{
		endpoint:      endpoint,
		labels:        labels,
		metrics:       metrics,
		clientLibrary: clientLibrary,
		httpClient: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
	}

	// Parse basic auth configuration
	basicAuth, err := parseBasicAuth(config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse basic auth: %w", err)
	}
	stream.basicAuth = basicAuth

	return stream, nil
}

// Load loads data to Prometheus using remote write protocol
func (p *PrometheusRemoteWriteStream) Load(ctx context.Context, results []*transform.TransformedResult) error {
	if len(results) == 0 {
		return nil
	}

	// Check if there's any CSV data to load
	if !hasCSVData(results) {
		return nil // Skip API call when no data to load
	}

	// Convert results to Prometheus remote write format
	timeSeries := p.convertToPrometheusTimeSeries(results)
	if len(timeSeries) == 0 {
		return nil
	}

	// Use different client library based on configuration
	switch p.clientLibrary {
	case "m3dbx":
		return p.loadWithM3DBX(ctx, timeSeries)
	default: // "native"
		return p.loadWithNative(ctx, timeSeries)
	}
}

// loadWithNative loads data using the native Prometheus remote write implementation
func (p *PrometheusRemoteWriteStream) loadWithNative(ctx context.Context, timeSeries []*prompb.TimeSeries) error {
	// Create WriteRequest
	writeRequest := &prompb.WriteRequest{}
	for _, ts := range timeSeries {
		writeRequest.Timeseries = append(writeRequest.Timeseries, *ts)
	}

	// Marshal to protobuf
	data, err := writeRequest.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal write request: %w", err)
	}

	// Compress with snappy
	compressed := snappy.Encode(nil, data)

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", p.endpoint, bytes.NewReader(compressed))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/x-protobuf")
	req.Header.Set("Content-Encoding", "snappy")
	req.Header.Set("X-Prometheus-Remote-Write-Version", "0.1.0")

	// Add basic auth header if configured
	if p.basicAuth != "" {
		req.Header.Set("Authorization", p.basicAuth)
	}

	// Send request
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("Prometheus remote write returned status %d", resp.StatusCode)
	}

	return nil
}

// loadWithM3DBX loads data using the m3dbx Prometheus remote client golang library
func (p *PrometheusRemoteWriteStream) loadWithM3DBX(ctx context.Context, timeSeries []*prompb.TimeSeries) error {
	// TODO: Implement m3dbx client integration
	// This would require importing the m3dbx Prometheus_remote_client_golang library
	// and using its client to perform the remote write operation

	// For now, return an error indicating the feature is not yet implemented
	return fmt.Errorf("m3dbx client library integration is not yet implemented - please use 'native' client_library or leave unspecified")

	// Example of what the implementation might look like:
	// 1. Create m3dbx client with endpoint and auth configuration
	// 2. Convert timeSeries to m3dbx format if needed
	// 3. Use m3dbx client to write the time series data
	// 4. Handle any m3dbx-specific errors and return appropriate error messages
}

// convertToPrometheusTimeSeries converts transformed results to Prometheus time series using CSV data
func (p *PrometheusRemoteWriteStream) convertToPrometheusTimeSeries(results []*transform.TransformedResult) []*prompb.TimeSeries {
	var timeSeries []*prompb.TimeSeries

	for _, result := range results {
		// Use CSV data to create time series if available and metrics are configured
		if len(result.CSVData) > 0 && len(p.metrics) > 0 {
			// Generate time series for each metric using CSV data
			for _, metric := range p.metrics {
				metricTimeSeries := p.createTimeSeriesForMetric(result.CSVData, metric)
				timeSeries = append(timeSeries, metricTimeSeries...)
			}
			continue
		}

		// Fallback to old behavior using TransformedData
		timestamp := result.Timestamp.UnixMilli()
		for key, value := range result.TransformedData {
			// Only include numeric values as metrics
			if numValue, ok := p.toFloat64(value); ok {
				// Create labels
				var labels []prompb.Label
				labels = append(labels, prompb.Label{Name: "__name__", Value: key})
				labels = append(labels, prompb.Label{Name: "source", Value: result.Source})

				// Add cluster name from metadata if available
				if clusterName, ok := safeString(result.Metadata["cluster_name"]); ok && clusterName != "" {
					labels = append(labels, prompb.Label{Name: "cluster", Value: clusterName})
				}

				// Add configured labels
				for labelKey, labelValue := range p.labels {
					labels = append(labels, prompb.Label{Name: labelKey, Value: labelValue})
				}

				// Create time series
				ts := &prompb.TimeSeries{
					Labels: labels,
					Samples: []prompb.Sample{
						{
							Value:     numValue,
							Timestamp: timestamp,
						},
					},
				}
				timeSeries = append(timeSeries, ts)
			}
		}
	}

	return timeSeries
}

// createTimeSeriesForMetric creates Prometheus remote write time series for a specific metric using CSV data
func (p *PrometheusRemoteWriteStream) createTimeSeriesForMetric(csvData [][]string, metric config.PrometheusMetricConfig) []*prompb.TimeSeries {
	var timeSeries []*prompb.TimeSeries

	// Group CSV rows by unique field combinations
	uniqueGroups := make(map[string][]map[string]interface{})

	for _, row := range csvData {
		// Check bounds for required columns
		if metric.Value >= len(row) || metric.Timestamp >= len(row) {
			continue // Skip rows that don't have required columns
		}

		// Create unique key from uniqueFieldsIndex with bounds checking
		var keyParts []string
		for _, idx := range metric.UniqueFieldsIndex {
			if idx >= 0 && idx < len(row) {
				keyParts = append(keyParts, row[idx])
			}
		}
		uniqueKey := strings.Join(keyParts, "|")

		// Parse value and timestamp with bounds checking
		value, ok := p.parseFloat(row[metric.Value])
		if !ok {
			continue
		}

		timestamp, ok := p.parseInt64(row[metric.Timestamp])
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

		var labels []prompb.Label
		labels = append(labels, prompb.Label{Name: "__name__", Value: metric.Name})

		// Add dynamic labels with bounds checking
		for _, label := range metric.Labels {
			if label.StaticValue != "" {
				labels = append(labels, prompb.Label{Name: label.LabelName, Value: label.StaticValue})
			} else if label.IndexInCSVData >= 0 && label.IndexInCSVData < len(row) {
				labels = append(labels, prompb.Label{Name: label.LabelName, Value: row[label.IndexInCSVData]})
			}
		}

		// Add configured labels
		for labelKey, labelValue := range p.labels {
			labels = append(labels, prompb.Label{Name: labelKey, Value: labelValue})
		}

		// Create samples array for this time series
		var samples []prompb.Sample
		for _, sample := range groupSamples {
			samples = append(samples, prompb.Sample{
				Value:     sample["value"].(float64),
				Timestamp: sample["timestamp"].(int64),
			})
		}

		// Create time series
		ts := &prompb.TimeSeries{
			Labels:  labels,
			Samples: samples,
		}

		timeSeries = append(timeSeries, ts)
	}

	return timeSeries
}

// parseFloat parses a string to float64 for Prometheus remote write stream
func (p *PrometheusRemoteWriteStream) parseFloat(s string) (float64, bool) {
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f, true
	}
	return 0, false
}

// parseInt64 parses a string to int64 for Prometheus remote write stream
func (p *PrometheusRemoteWriteStream) parseInt64(s string) (int64, bool) {
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return int64(f), true
	}
	return 0, false
}

// toFloat64 converts a value to float64 if possible
func (p *PrometheusRemoteWriteStream) toFloat64(value interface{}) (float64, bool) {
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

// Close closes the Prometheus remote write stream
func (p *PrometheusRemoteWriteStream) Close() error {
	return nil
}

// GetType returns the stream type
func (p *PrometheusRemoteWriteStream) GetType() string {
	return "prometheus_remote_write"
}

// writeLoadDebugInfo writes debug information for the load stage
func (l *Loader) writeLoadDebugInfo(results []*transform.TransformedResult, pipelineName string, debugConfig config.LoadDebugConfig) error {
	// Set default path if not specified
	debugPath := debugConfig.Path
	if debugPath == "" {
		debugPath = "debug"
	}

	// Create debug directory if it doesn't exist
	if err := os.MkdirAll(debugPath, 0755); err != nil {
		return fmt.Errorf("failed to create debug directory: %w", err)
	}

	timestamp := time.Now().Format("20060102_150405")

	// Write input debug info if enabled
	if debugConfig.Input {
		inputData := map[string]interface{}{
			"timestamp":     time.Now().Format(time.RFC3339),
			"pipeline":      pipelineName,
			"stage":         "load",
			"type":          "input",
			"results_count": len(results),
			"results":       results,
		}

		inputJSON, err := json.MarshalIndent(inputData, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal input debug data: %w", err)
		}

		inputFile := filepath.Join(debugPath, fmt.Sprintf("%s_load_input_%s.json", pipelineName, timestamp))
		if err := os.WriteFile(inputFile, inputJSON, 0644); err != nil {
			return fmt.Errorf("failed to write input debug file: %w", err)
		}

		fmt.Printf("Load input debug info written to: %s\n", inputFile)
	}

	return nil
}

// loadWithStreamDebug loads data to a stream with per-stream debug information
func (l *Loader) loadWithStreamDebug(ctx context.Context, results []*transform.TransformedResult, stream Stream, pipelineName, streamName string, debugConfig config.LoadDebugConfig) error {
	// Set default path if not specified
	debugPath := debugConfig.Path
	if debugPath == "" {
		debugPath = "debug"
	}

	timestamp := time.Now().Format("20060102_150405")

	// Write input debug info if enabled (per stream)
	if debugConfig.Input {
		if err := l.writeStreamInputDebugInfo(results, pipelineName, streamName, debugPath, timestamp); err != nil {
			fmt.Printf("Warning: Failed to write stream input debug info: %v\n", err)
		}
	}

	// Write API call debug info if enabled
	if debugConfig.APICall {
		if err := l.writeStreamAPICallDebugInfo(stream, results, pipelineName, streamName, debugPath, timestamp); err != nil {
			fmt.Printf("Warning: Failed to write API call debug info: %v\n", err)
		}
	}

	// Set pipeline name for DebugStream if applicable
	if debugStream, ok := stream.(*DebugStream); ok {
		debugStream.SetPipelineName(pipelineName)
	}

	// Perform the actual load
	err := stream.Load(ctx, results)

	// Write API response debug info if enabled
	if debugConfig.APIResponse {
		if err := l.writeStreamAPIResponseDebugInfo(stream, err, pipelineName, streamName, debugPath, timestamp); err != nil {
			fmt.Printf("Warning: Failed to write API response debug info: %v\n", err)
		}
	}

	return err
}

// loadWithDebug loads data to a stream with debug information (legacy method for backward compatibility)
func (l *Loader) loadWithDebug(ctx context.Context, results []*transform.TransformedResult, stream Stream, pipelineName string, debugConfig config.LoadDebugConfig) error {
	return l.loadWithStreamDebug(ctx, results, stream, pipelineName, stream.GetType(), debugConfig)
}

// writeAPICallDebugInfo writes debug information about API calls
func (l *Loader) writeAPICallDebugInfo(stream Stream, results []*transform.TransformedResult, pipelineName, debugPath, timestamp string) error {
	// Create debug directory if it doesn't exist
	if err := os.MkdirAll(debugPath, 0755); err != nil {
		return fmt.Errorf("failed to create debug directory: %w", err)
	}

	apiCallData := map[string]interface{}{
		"timestamp":     time.Now().Format(time.RFC3339),
		"pipeline":      pipelineName,
		"stage":         "load",
		"type":          "api_call",
		"stream_type":   stream.GetType(),
		"results_count": len(results),
	}

	// Add stream-specific debug information
	switch s := stream.(type) {
	case *GEMStream:
		apiCallData["endpoint"] = s.endpoint
		apiCallData["headers"] = map[string]string{
			"Content-Type":                      "application/json",
			"X-Prometheus-Remote-Write-Version": "0.1.0",
		}
		apiCallData["method"] = "POST"
		apiCallData["insecure_tls"] = false // GEM stream doesn't expose this directly

		// Generate equivalent curl command
		curlCmd := fmt.Sprintf("curl -X POST '%s' \\\n", s.endpoint)
		curlCmd += "  -H 'Content-Type: application/json' \\\n"
		curlCmd += "  -H 'X-Prometheus-Remote-Write-Version: 0.1.0' \\\n"
		curlCmd += "  -d '<JSON_DATA>'"
		apiCallData["curl_equivalent"] = curlCmd

	case *OTELStream:
		apiCallData["endpoint"] = s.endpoint
		apiCallData["headers"] = map[string]string{
			"Content-Type": "application/json",
		}
		apiCallData["method"] = "POST"

		// Generate equivalent curl command
		curlCmd := fmt.Sprintf("curl -X POST '%s' \\\n", s.endpoint)
		curlCmd += "  -H 'Content-Type: application/json' \\\n"
		curlCmd += "  -d '<JSON_DATA>'"
		apiCallData["curl_equivalent"] = curlCmd

	case *PrometheusStream:
		apiCallData["endpoint"] = s.endpoint
		headers := map[string]string{
			"Content-Type": "text/plain",
		}
		if s.basicAuth != "" {
			headers["Authorization"] = "Basic <ENCODED_CREDENTIALS>"
		}
		apiCallData["headers"] = headers
		apiCallData["method"] = "POST"

		// Generate equivalent curl command
		curlCmd := fmt.Sprintf("curl -X POST '%s' \\\n", s.endpoint)
		curlCmd += "  -H 'Content-Type: text/plain' \\\n"
		if s.basicAuth != "" {
			curlCmd += "  -H 'Authorization: Basic <ENCODED_CREDENTIALS>' \\\n"
		}
		curlCmd += "  -d '<PROMETHEUS_DATA>'"
		apiCallData["curl_equivalent"] = curlCmd

	case *PrometheusRemoteWriteStream:
		apiCallData["endpoint"] = s.endpoint
		headers := map[string]string{
			"Content-Type":                      "application/x-protobuf",
			"Content-Encoding":                  "snappy",
			"X-Prometheus-Remote-Write-Version": "0.1.0",
		}
		if s.basicAuth != "" {
			headers["Authorization"] = "Basic <ENCODED_CREDENTIALS>"
		}
		apiCallData["headers"] = headers
		apiCallData["method"] = "POST"

		// Generate equivalent curl command
		curlCmd := fmt.Sprintf("curl -X POST '%s' \\\n", s.endpoint)
		curlCmd += "  -H 'Content-Type: application/x-protobuf' \\\n"
		curlCmd += "  -H 'Content-Encoding: snappy' \\\n"
		curlCmd += "  -H 'X-Prometheus-Remote-Write-Version: 0.1.0' \\\n"
		if s.basicAuth != "" {
			curlCmd += "  -H 'Authorization: Basic <ENCODED_CREDENTIALS>' \\\n"
		}
		curlCmd += "  --data-binary '<PROTOBUF_DATA>'"
		apiCallData["curl_equivalent"] = curlCmd

	default:
		apiCallData["note"] = "Stream type does not make HTTP API calls"
	}

	apiCallJSON, err := json.MarshalIndent(apiCallData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal API call debug data: %w", err)
	}

	apiCallFile := filepath.Join(debugPath, fmt.Sprintf("%s_load_api_call_%s.json", pipelineName, timestamp))
	if err := os.WriteFile(apiCallFile, apiCallJSON, 0644); err != nil {
		return fmt.Errorf("failed to write API call debug file: %w", err)
	}

	fmt.Printf("Load API call debug info written to: %s\n", apiCallFile)
	return nil
}

// writeAPIResponseDebugInfo writes debug information about API responses
func (l *Loader) writeAPIResponseDebugInfo(stream Stream, loadErr error, pipelineName, debugPath, timestamp string) error {
	// Create debug directory if it doesn't exist
	if err := os.MkdirAll(debugPath, 0755); err != nil {
		return fmt.Errorf("failed to create debug directory: %w", err)
	}

	apiResponseData := map[string]interface{}{
		"timestamp":   time.Now().Format(time.RFC3339),
		"pipeline":    pipelineName,
		"stage":       "load",
		"type":        "api_response",
		"stream_type": stream.GetType(),
		"success":     loadErr == nil,
	}

	if loadErr != nil {
		apiResponseData["error"] = loadErr.Error()
		apiResponseData["status"] = "error"
	} else {
		apiResponseData["status"] = "success"
	}

	// Add stream-specific response information
	switch stream.GetType() {
	case "gem", "otel", "prometheus", "prometheus_remote_write":
		if loadErr != nil {
			// Try to extract HTTP status code from error message
			errorMsg := loadErr.Error()
			if strings.Contains(errorMsg, "returned status") {
				apiResponseData["http_status_extracted"] = errorMsg
			}
		} else {
			apiResponseData["http_status"] = "2xx (success)"
		}
	case "debug", "csv":
		apiResponseData["note"] = "File-based stream, no HTTP response"
	}

	apiResponseJSON, err := json.MarshalIndent(apiResponseData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal API response debug data: %w", err)
	}

	apiResponseFile := filepath.Join(debugPath, fmt.Sprintf("%s_load_api_response_%s.json", pipelineName, timestamp))
	if err := os.WriteFile(apiResponseFile, apiResponseJSON, 0644); err != nil {
		return fmt.Errorf("failed to write API response debug file: %w", err)
	}

	fmt.Printf("Load API response debug info written to: %s\n", apiResponseFile)
	return nil
}

// writeStreamInputDebugInfo writes per-stream input debug information
func (l *Loader) writeStreamInputDebugInfo(results []*transform.TransformedResult, pipelineName, streamName, debugPath, timestamp string) error {
	// Create debug directory if it doesn't exist
	if err := os.MkdirAll(debugPath, 0755); err != nil {
		return fmt.Errorf("failed to create debug directory: %w", err)
	}

	inputData := map[string]interface{}{
		"timestamp":     time.Now().Format(time.RFC3339),
		"pipeline":      pipelineName,
		"stream":        streamName,
		"stage":         "load",
		"type":          "input",
		"results_count": len(results),
		"results":       results,
	}

	inputJSON, err := json.MarshalIndent(inputData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal stream input debug data: %w", err)
	}

	inputFile := filepath.Join(debugPath, fmt.Sprintf("%s_%s_load_input_%s.json", pipelineName, streamName, timestamp))
	if err := os.WriteFile(inputFile, inputJSON, 0644); err != nil {
		return fmt.Errorf("failed to write stream input debug file: %w", err)
	}

	fmt.Printf("Stream %s load input debug info written to: %s\n", streamName, inputFile)
	return nil
}

// writeStreamAPICallDebugInfo writes per-stream API call debug information
func (l *Loader) writeStreamAPICallDebugInfo(stream Stream, results []*transform.TransformedResult, pipelineName, streamName, debugPath, timestamp string) error {
	// Create debug directory if it doesn't exist
	if err := os.MkdirAll(debugPath, 0755); err != nil {
		return fmt.Errorf("failed to create debug directory: %w", err)
	}

	apiCallData := map[string]interface{}{
		"timestamp":     time.Now().Format(time.RFC3339),
		"pipeline":      pipelineName,
		"stream":        streamName,
		"stage":         "load",
		"type":          "api_call",
		"stream_type":   stream.GetType(),
		"results_count": len(results),
	}

	// Add stream-specific debug information
	switch s := stream.(type) {
	case *GEMStream:
		apiCallData["endpoint"] = s.endpoint
		apiCallData["headers"] = map[string]string{
			"Content-Type":                      "application/json",
			"X-Prometheus-Remote-Write-Version": "0.1.0",
		}
		apiCallData["method"] = "POST"
		apiCallData["insecure_tls"] = false // GEM stream doesn't expose this directly

		// Generate equivalent curl command
		curlCmd := fmt.Sprintf("curl -X POST '%s' \\\n", s.endpoint)
		curlCmd += "  -H 'Content-Type: application/json' \\\n"
		curlCmd += "  -H 'X-Prometheus-Remote-Write-Version: 0.1.0' \\\n"
		curlCmd += "  -d '<JSON_DATA>'"
		apiCallData["curl_equivalent"] = curlCmd

	case *OTELStream:
		apiCallData["endpoint"] = s.endpoint
		apiCallData["headers"] = map[string]string{
			"Content-Type": "application/json",
		}
		apiCallData["method"] = "POST"

		// Generate equivalent curl command
		curlCmd := fmt.Sprintf("curl -X POST '%s' \\\n", s.endpoint)
		curlCmd += "  -H 'Content-Type: application/json' \\\n"
		curlCmd += "  -d '<JSON_DATA>'"
		apiCallData["curl_equivalent"] = curlCmd

	case *PrometheusStream:
		apiCallData["endpoint"] = s.endpoint
		headers := map[string]string{
			"Content-Type": "text/plain",
		}
		if s.basicAuth != "" {
			headers["Authorization"] = "Basic <ENCODED_CREDENTIALS>"
		}
		apiCallData["headers"] = headers
		apiCallData["method"] = "POST"

		// Generate equivalent curl command
		curlCmd := fmt.Sprintf("curl -X POST '%s' \\\n", s.endpoint)
		curlCmd += "  -H 'Content-Type: text/plain' \\\n"
		if s.basicAuth != "" {
			curlCmd += "  -H 'Authorization: Basic <ENCODED_CREDENTIALS>' \\\n"
		}
		curlCmd += "  -d '<PROMETHEUS_DATA>'"
		apiCallData["curl_equivalent"] = curlCmd

	case *PrometheusRemoteWriteStream:
		apiCallData["endpoint"] = s.endpoint
		headers := map[string]string{
			"Content-Type":                      "application/x-protobuf",
			"Content-Encoding":                  "snappy",
			"X-Prometheus-Remote-Write-Version": "0.1.0",
		}
		if s.basicAuth != "" {
			headers["Authorization"] = "Basic <ENCODED_CREDENTIALS>"
		}
		apiCallData["headers"] = headers
		apiCallData["method"] = "POST"

		// Generate equivalent curl command
		curlCmd := fmt.Sprintf("curl -X POST '%s' \\\n", s.endpoint)
		curlCmd += "  -H 'Content-Type: application/x-protobuf' \\\n"
		curlCmd += "  -H 'Content-Encoding: snappy' \\\n"
		curlCmd += "  -H 'X-Prometheus-Remote-Write-Version: 0.1.0' \\\n"
		if s.basicAuth != "" {
			curlCmd += "  -H 'Authorization: Basic <ENCODED_CREDENTIALS>' \\\n"
		}
		curlCmd += "  --data-binary '<PROTOBUF_DATA>'"
		apiCallData["curl_equivalent"] = curlCmd

	default:
		apiCallData["note"] = "Stream type does not make HTTP API calls"
	}

	apiCallJSON, err := json.MarshalIndent(apiCallData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal stream API call debug data: %w", err)
	}

	apiCallFile := filepath.Join(debugPath, fmt.Sprintf("%s_%s_load_api_call_%s.json", pipelineName, streamName, timestamp))
	if err := os.WriteFile(apiCallFile, apiCallJSON, 0644); err != nil {
		return fmt.Errorf("failed to write stream API call debug file: %w", err)
	}

	fmt.Printf("Stream %s load API call debug info written to: %s\n", streamName, apiCallFile)
	return nil
}

// writeStreamAPIResponseDebugInfo writes per-stream API response debug information
func (l *Loader) writeStreamAPIResponseDebugInfo(stream Stream, loadErr error, pipelineName, streamName, debugPath, timestamp string) error {
	// Create debug directory if it doesn't exist
	if err := os.MkdirAll(debugPath, 0755); err != nil {
		return fmt.Errorf("failed to create debug directory: %w", err)
	}

	apiResponseData := map[string]interface{}{
		"timestamp":   time.Now().Format(time.RFC3339),
		"pipeline":    pipelineName,
		"stream":      streamName,
		"stage":       "load",
		"type":        "api_response",
		"stream_type": stream.GetType(),
		"success":     loadErr == nil,
	}

	if loadErr != nil {
		apiResponseData["error"] = loadErr.Error()
		apiResponseData["status"] = "error"
	} else {
		apiResponseData["status"] = "success"
	}

	// Add stream-specific response information
	switch stream.GetType() {
	case "gem", "otel", "prometheus", "prometheus_remote_write":
		if loadErr != nil {
			// Try to extract HTTP status code from error message
			errorMsg := loadErr.Error()
			if strings.Contains(errorMsg, "returned status") {
				apiResponseData["http_status_extracted"] = errorMsg
			}
		} else {
			apiResponseData["http_status"] = "2xx (success)"
		}
	case "debug", "csv":
		apiResponseData["note"] = "File-based stream, no HTTP response"
	}

	apiResponseJSON, err := json.MarshalIndent(apiResponseData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal stream API response debug data: %w", err)
	}

	apiResponseFile := filepath.Join(debugPath, fmt.Sprintf("%s_%s_load_api_response_%s.json", pipelineName, streamName, timestamp))
	if err := os.WriteFile(apiResponseFile, apiResponseJSON, 0644); err != nil {
		return fmt.Errorf("failed to write stream API response debug file: %w", err)
	}

	fmt.Printf("Stream %s load API response debug info written to: %s\n", streamName, apiResponseFile)
	return nil
}
