package config

import (
	"time"
)

// Config represents the main configuration structure
type Config struct {
	Pipelines []PipelineConfig `json:"pipelines" yaml:"pipelines"`
	Global    GlobalConfig     `json:"global" yaml:"global"`
}

// PipelineConfig represents a single ETL pipeline configuration
type PipelineConfig struct {
	Name      string          `json:"name" yaml:"name"`
	Enabled   bool            `json:"enabled" yaml:"enabled"`
	Interval  time.Duration   `json:"interval" yaml:"interval"`
	Extract   ExtractConfig   `json:"extract" yaml:"extract"`
	Transform TransformConfig `json:"transform" yaml:"transform"`
	Load      LoadConfig      `json:"load" yaml:"load"`
}

// ExtractConfig contains extraction configuration
type ExtractConfig struct {
	ElasticsearchQuery string        `json:"elasticsearch_query" yaml:"elasticsearch_query"`
	URLs               []string      `json:"urls" yaml:"urls"`
	ClusterNames       []string      `json:"cluster_names" yaml:"cluster_names"`
	AuthHeaders        []string      `json:"auth_headers,omitempty" yaml:"auth_headers,omitempty"`
	AdditionalHeaders  [][]string    `json:"additional_headers,omitempty" yaml:"additional_headers,omitempty"`
	JSONPaths          []string      `json:"json_paths" yaml:"json_paths"`
	Interval           time.Duration `json:"interval" yaml:"interval"`
	Timeout            time.Duration `json:"timeout" yaml:"timeout"`
	MaxRetries         int           `json:"max_retries" yaml:"max_retries"`
	StartTime          string        `json:"start_time,omitempty" yaml:"start_time,omitempty"`
	EndTime            string        `json:"end_time,omitempty" yaml:"end_time,omitempty"`
	InsecureTLS        bool          `json:"insecure_tls,omitempty" yaml:"insecure_tls,omitempty"`
}

// TransformConfig contains transformation configuration
type TransformConfig struct {
	Stateless              bool                       `json:"stateless" yaml:"stateless"`
	SubstituteZerosForNull bool                       `json:"substitute_zeros_for_null" yaml:"substitute_zeros_for_null"`
	PreviousResultsSets    int                        `json:"previous_results_sets" yaml:"previous_results_sets"`
	ConversionFunctions    []ConversionFunctionConfig `json:"conversion_functions" yaml:"conversion_functions"`
}

// ConversionFunctionConfig defines field conversion functions
type ConversionFunctionConfig struct {
	Field    string `json:"field" yaml:"field"`
	Function string `json:"function" yaml:"function"` // convert_type, convert_to_kb, convert_to_mb, convert_to_gb
	FromType string `json:"from_type,omitempty" yaml:"from_type,omitempty"`
	ToType   string `json:"to_type,omitempty" yaml:"to_type,omitempty"`
	FromUnit string `json:"from_unit,omitempty" yaml:"from_unit,omitempty"`
	ToUnit   string `json:"to_unit,omitempty" yaml:"to_unit,omitempty"`
}

// LoadConfig contains load configuration
type LoadConfig struct {
	Streams []StreamConfig `json:"streams" yaml:"streams"`
}

// StreamConfig defines a single load stream
type StreamConfig struct {
	Type        string                 `json:"type" yaml:"type"` // gem, otel, prometheus
	Config      map[string]interface{} `json:"config" yaml:"config"`
	BasicAuth   *BasicAuthConfig       `json:"basic_auth,omitempty" yaml:"basic_auth,omitempty"`
	InsecureTLS bool                   `json:"insecure_tls,omitempty" yaml:"insecure_tls,omitempty"`
}

// BasicAuthConfig defines basic authentication configuration
type BasicAuthConfig struct {
	Username string `json:"username" yaml:"username"`
	Password string `json:"password" yaml:"password"`
}

// GlobalConfig contains global application settings
type GlobalConfig struct {
	ResourceLimits ResourceLimits `json:"resource_limits" yaml:"resource_limits"`
	Metrics        MetricsConfig  `json:"metrics" yaml:"metrics"`
	Logging        LoggingConfig  `json:"logging" yaml:"logging"`
}

// ResourceLimits defines resource consumption limits
type ResourceLimits struct {
	MaxMemoryMB    int `json:"max_memory_mb" yaml:"max_memory_mb"`
	MaxCPUPercent  int `json:"max_cpu_percent" yaml:"max_cpu_percent"`
	MaxGoroutines  int `json:"max_goroutines" yaml:"max_goroutines"`
	MaxConnections int `json:"max_connections" yaml:"max_connections"`
}

// MetricsConfig defines metrics collection settings
type MetricsConfig struct {
	Enabled  bool          `json:"enabled" yaml:"enabled"`
	Port     int           `json:"port" yaml:"port"`
	Path     string        `json:"path" yaml:"path"`
	Interval time.Duration `json:"interval" yaml:"interval"`
}

// LoggingConfig defines logging settings
type LoggingConfig struct {
	Level  string `json:"level" yaml:"level"`
	Format string `json:"format" yaml:"format"` // json, text
	Output string `json:"output" yaml:"output"` // stdout, file
	File   string `json:"file,omitempty" yaml:"file,omitempty"`
}
