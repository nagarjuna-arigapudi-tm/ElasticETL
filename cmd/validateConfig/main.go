package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"elasticetl/pkg/config"

	"github.com/robfig/cron/v3"
	"gopkg.in/yaml.v2"
)

// ValidationError represents a single validation error
type ValidationError struct {
	Path       string `json:"path"`
	Field      string `json:"field"`
	Value      string `json:"value"`
	Message    string `json:"message"`
	Severity   string `json:"severity"` // "error", "warning", "info"
	Suggestion string `json:"suggestion,omitempty"`
	ErrorCode  string `json:"error_code"`
}

// ValidationResult contains all validation results
type ValidationResult struct {
	Valid      bool              `json:"valid"`
	Errors     []ValidationError `json:"errors"`
	Warnings   []ValidationError `json:"warnings"`
	Info       []ValidationError `json:"info"`
	Summary    ValidationSummary `json:"summary"`
	ConfigPath string            `json:"config_path"`
	ConfigType string            `json:"config_type"`
}

// ValidationSummary provides a summary of validation results
type ValidationSummary struct {
	TotalErrors    int `json:"total_errors"`
	TotalWarnings  int `json:"total_warnings"`
	TotalInfo      int `json:"total_info"`
	PipelinesCount int `json:"pipelines_count"`
	ValidPipelines int `json:"valid_pipelines"`
}

// ConfigValidator handles configuration validation
type ConfigValidator struct {
	result *ValidationResult
}

func main() {
	var configPath string
	var outputFormat string
	var verbose bool
	var strictMode bool

	flag.StringVar(&configPath, "config", "", "Path to configuration file (required)")
	flag.StringVar(&outputFormat, "format", "text", "Output format: text, json, yaml")
	flag.BoolVar(&verbose, "verbose", false, "Enable verbose output")
	flag.BoolVar(&strictMode, "strict", false, "Enable strict validation mode")
	flag.Parse()

	if configPath == "" {
		fmt.Fprintf(os.Stderr, "Error: -config flag is required\n")
		flag.Usage()
		os.Exit(1)
	}

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: Configuration file does not exist: %s\n", configPath)
		os.Exit(1)
	}

	// Create validator
	validator := NewConfigValidator()

	// Validate configuration
	result, err := validator.ValidateConfigFile(configPath, strictMode)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to validate configuration: %v\n", err)
		os.Exit(1)
	}

	// Output results
	if err := outputResults(result, outputFormat, verbose); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to output results: %v\n", err)
		os.Exit(1)
	}

	// Exit with appropriate code
	if !result.Valid {
		os.Exit(1)
	}
}

// NewConfigValidator creates a new configuration validator
func NewConfigValidator() *ConfigValidator {
	return &ConfigValidator{
		result: &ValidationResult{
			Valid:    true,
			Errors:   make([]ValidationError, 0),
			Warnings: make([]ValidationError, 0),
			Info:     make([]ValidationError, 0),
		},
	}
}

// ValidateConfigFile validates a configuration file
func (v *ConfigValidator) ValidateConfigFile(configPath string, strictMode bool) (*ValidationResult, error) {
	v.result.ConfigPath = configPath

	// Determine config type
	ext := strings.ToLower(filepath.Ext(configPath))
	switch ext {
	case ".json":
		v.result.ConfigType = "JSON"
	case ".yaml", ".yml":
		v.result.ConfigType = "YAML"
	default:
		return nil, fmt.Errorf("unsupported configuration file format: %s (supported: .json, .yaml, .yml)", ext)
	}

	// Read configuration file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read configuration file: %w", err)
	}

	// Parse configuration
	var cfg config.Config
	if err := v.parseConfig(data, ext, &cfg); err != nil {
		v.addError("", "file", "", fmt.Sprintf("Failed to parse configuration file: %v", err), "PARSE_ERROR", "")
		v.result.Valid = false
		v.updateSummary()
		return v.result, nil
	}

	// Validate configuration structure
	v.validateConfig(&cfg, strictMode)

	// Update summary
	v.updateSummary()

	return v.result, nil
}

// parseConfig parses configuration data based on file extension
func (v *ConfigValidator) parseConfig(data []byte, ext string, cfg *config.Config) error {
	switch ext {
	case ".json":
		return json.Unmarshal(data, cfg)
	case ".yaml", ".yml":
		return yaml.Unmarshal(data, cfg)
	default:
		return fmt.Errorf("unsupported file extension: %s", ext)
	}
}

// validateConfig validates the entire configuration
func (v *ConfigValidator) validateConfig(cfg *config.Config, strictMode bool) {
	// Validate global configuration
	v.validateGlobalConfig(&cfg.Global, strictMode)

	// Validate pipelines
	if len(cfg.Pipelines) == 0 {
		v.addError("pipelines", "array", "[]", "At least one pipeline must be configured", "NO_PIPELINES", "Add at least one pipeline configuration")
	}

	validPipelines := 0
	for i, pipeline := range cfg.Pipelines {
		pipelinePath := fmt.Sprintf("pipelines[%d]", i)
		if v.validatePipeline(&pipeline, pipelinePath, strictMode) {
			validPipelines++
		}
	}

	v.result.Summary.ValidPipelines = validPipelines
	v.result.Summary.PipelinesCount = len(cfg.Pipelines)

	// Check for duplicate pipeline names
	v.checkDuplicatePipelineNames(cfg.Pipelines)
}

// validateGlobalConfig validates global configuration
func (v *ConfigValidator) validateGlobalConfig(global *config.GlobalConfig, strictMode bool) {
	path := "global"

	// Validate resource limits
	v.validateResourceLimits(&global.ResourceLimits, path+".resource_limits", strictMode)

	// Validate metrics configuration
	v.validateMetricsConfig(&global.Metrics, path+".metrics", strictMode)

	// Validate logging configuration
	v.validateLoggingConfig(&global.Logging, path+".logging", strictMode)
}

// validateResourceLimits validates resource limits configuration
func (v *ConfigValidator) validateResourceLimits(limits *config.ResourceLimits, path string, strictMode bool) {
	if limits.MaxMemoryMB <= 0 {
		v.addError(path, "max_memory_mb", fmt.Sprintf("%d", limits.MaxMemoryMB), "Memory limit must be positive", "INVALID_MEMORY_LIMIT", "Set a positive value (e.g., 512)")
	} else if limits.MaxMemoryMB < 128 {
		v.addWarning(path, "max_memory_mb", fmt.Sprintf("%d", limits.MaxMemoryMB), "Memory limit is very low, may cause performance issues", "LOW_MEMORY_LIMIT", "Consider setting at least 128MB")
	}

	if limits.MaxCPUPercent <= 0 || limits.MaxCPUPercent > 100 {
		v.addError(path, "max_cpu_percent", fmt.Sprintf("%d", limits.MaxCPUPercent), "CPU limit must be between 1 and 100", "INVALID_CPU_LIMIT", "Set a value between 1 and 100")
	}

	if limits.MaxGoroutines <= 0 {
		v.addError(path, "max_goroutines", fmt.Sprintf("%d", limits.MaxGoroutines), "Goroutine limit must be positive", "INVALID_GOROUTINE_LIMIT", "Set a positive value (e.g., 1000)")
	}

	if limits.MaxConnections <= 0 {
		v.addError(path, "max_connections", fmt.Sprintf("%d", limits.MaxConnections), "Connection limit must be positive", "INVALID_CONNECTION_LIMIT", "Set a positive value (e.g., 100)")
	}
}

// validateMetricsConfig validates metrics configuration
func (v *ConfigValidator) validateMetricsConfig(metrics *config.MetricsConfig, path string, strictMode bool) {
	if metrics.Enabled {
		if metrics.Port <= 0 || metrics.Port > 65535 {
			v.addError(path, "port", fmt.Sprintf("%d", metrics.Port), "Port must be between 1 and 65535", "INVALID_PORT", "Use a valid port number (e.g., 8080)")
		} else if metrics.Port < 1024 && strictMode {
			v.addWarning(path, "port", fmt.Sprintf("%d", metrics.Port), "Using privileged port (< 1024) may require elevated permissions", "PRIVILEGED_PORT", "Consider using a port >= 1024")
		}

		if metrics.Path == "" {
			v.addWarning(path, "path", "", "Metrics path is empty, will use default", "EMPTY_METRICS_PATH", "Set explicit path (e.g., '/metrics')")
		}

		if metrics.Interval <= 0 {
			v.addWarning(path, "interval", metrics.Interval.String(), "Metrics interval is not set, will use default", "NO_METRICS_INTERVAL", "Set explicit interval (e.g., '30s')")
		}
	}
}

// validateLoggingConfig validates logging configuration
func (v *ConfigValidator) validateLoggingConfig(logging *config.LoggingConfig, path string, strictMode bool) {
	validLevels := []string{"debug", "info", "warn", "error"}
	if !contains(validLevels, logging.Level) {
		v.addError(path, "level", logging.Level, fmt.Sprintf("Invalid log level, must be one of: %s", strings.Join(validLevels, ", ")), "INVALID_LOG_LEVEL", "Use 'info' for production")
	}

	validFormats := []string{"json", "text"}
	if !contains(validFormats, logging.Format) {
		v.addError(path, "format", logging.Format, fmt.Sprintf("Invalid log format, must be one of: %s", strings.Join(validFormats, ", ")), "INVALID_LOG_FORMAT", "Use 'json' for production")
	}

	validOutputs := []string{"stdout", "stderr", "file"}
	if !contains(validOutputs, logging.Output) && !strings.HasPrefix(logging.Output, "/") {
		v.addError(path, "output", logging.Output, "Invalid log output, must be 'stdout', 'stderr', 'file', or a file path", "INVALID_LOG_OUTPUT", "Use 'stdout' for containers")
	}

	if logging.Output == "file" && logging.File == "" {
		v.addError(path, "file", "", "Log file path is required when output is 'file'", "MISSING_LOG_FILE", "Specify a file path")
	}
}

// validatePipeline validates a single pipeline configuration
func (v *ConfigValidator) validatePipeline(pipeline *config.PipelineConfig, path string, strictMode bool) bool {
	valid := true

	// Validate pipeline name
	if pipeline.Name == "" {
		v.addError(path, "name", "", "Pipeline name is required", "MISSING_PIPELINE_NAME", "Provide a unique pipeline name")
		valid = false
	} else if !isValidPipelineName(pipeline.Name) {
		v.addError(path, "name", pipeline.Name, "Pipeline name contains invalid characters", "INVALID_PIPELINE_NAME", "Use alphanumeric characters, hyphens, and underscores only")
		valid = false
	}

	// Validate scheduling
	if !v.validateScheduleConfig(&pipeline.Schedule, pipeline.Interval, path+".schedule", strictMode) {
		valid = false
	}

	// Validate retry interval
	if pipeline.RetryInterval < 0 {
		v.addError(path, "retryInterval", pipeline.RetryInterval.String(), "Retry interval cannot be negative", "NEGATIVE_RETRY_INTERVAL", "Set to 0 to disable retry or positive duration")
		valid = false
	}

	// Validate extract configuration
	if !v.validateExtractConfig(&pipeline.Extract, path+".extract", strictMode) {
		valid = false
	}

	// Validate transform configuration
	if !v.validateTransformConfig(&pipeline.Transform, path+".transform", strictMode) {
		valid = false
	}

	// Validate load configuration
	if !v.validateLoadConfig(&pipeline.Load, path+".load", strictMode) {
		valid = false
	}

	return valid
}

// validateScheduleConfig validates schedule configuration
func (v *ConfigValidator) validateScheduleConfig(schedule *config.ScheduleConfig, deprecatedInterval time.Duration, path string, strictMode bool) bool {
	valid := true

	// Check if both interval and cron are specified
	if schedule.Interval > 0 && schedule.CronSchedule != "" {
		v.addInfo(path, "interval+cronSchedule", "", "Both interval and cron schedule specified, cron takes precedence", "BOTH_SCHEDULE_TYPES", "")
	}

	// Validate cron schedule if specified
	if schedule.CronSchedule != "" {
		if _, err := cron.ParseStandard(schedule.CronSchedule); err != nil {
			v.addError(path, "cronSchedule", schedule.CronSchedule, fmt.Sprintf("Invalid cron expression: %v", err), "INVALID_CRON", "Use standard cron format (e.g., '0 */5 * * * *')")
			valid = false
		}
	} else if schedule.Interval <= 0 {
		// Check deprecated interval field for backward compatibility
		if deprecatedInterval > 0 {
			v.addWarning(path, "interval", "", "Using deprecated 'interval' field, migrate to 'schedule.interval'", "DEPRECATED_INTERVAL", "Move interval to schedule.interval")
		} else {
			v.addError(path, "interval", "0", "Either schedule.interval must be positive or schedule.cronSchedule must be specified", "NO_SCHEDULE", "Set schedule.interval or schedule.cronSchedule")
			valid = false
		}
	}

	// Validate interval if specified
	if schedule.Interval > 0 && schedule.Interval < time.Second {
		v.addWarning(path, "interval", schedule.Interval.String(), "Very short interval may cause performance issues", "SHORT_INTERVAL", "Consider using at least 1 second")
	}

	// Validate start time format if specified
	if schedule.StartTime != "" {
		if !isValidTimeFormat(schedule.StartTime) {
			v.addError(path, "startTime", schedule.StartTime, "Invalid time format, expected HH:MM:SS", "INVALID_START_TIME", "Use format like '09:30:00'")
			valid = false
		}
	}

	return valid
}

// validateExtractConfig validates extract configuration
func (v *ConfigValidator) validateExtractConfig(extract *config.ExtractConfig, path string, strictMode bool) bool {
	valid := true

	// Validate query
	if extract.Query == "" {
		v.addError(path, "query", "", "Query is required", "MISSING_QUERY", "Provide an Elasticsearch DSL query")
		valid = false
	} else {
		// Try to parse as JSON to validate syntax
		var queryObj interface{}
		if err := json.Unmarshal([]byte(extract.Query), &queryObj); err != nil {
			v.addWarning(path, "query", "", "Query is not valid JSON, ensure it's properly formatted", "INVALID_QUERY_JSON", "Validate JSON syntax")
		}
	}

	// Validate URLs
	if len(extract.URLs) == 0 {
		v.addError(path, "urls", "[]", "At least one URL is required", "NO_URLS", "Add at least one Elasticsearch endpoint URL")
		valid = false
	} else {
		for i, urlStr := range extract.URLs {
			if urlStr == "" {
				v.addError(path, fmt.Sprintf("urls[%d]", i), "", "URL cannot be empty", "EMPTY_URL", "Provide a valid URL")
				valid = false
			} else if !isValidURL(urlStr) {
				v.addError(path, fmt.Sprintf("urls[%d]", i), urlStr, "Invalid URL format", "INVALID_URL", "Use format like 'https://elasticsearch:9200/index/_search'")
				valid = false
			}
		}
	}

	// Validate cluster names
	if len(extract.ClusterNames) == 0 {
		v.addError(path, "cluster_names", "[]", "At least one cluster name is required", "NO_CLUSTER_NAMES", "Add cluster names corresponding to URLs")
		valid = false
	} else {
		for i, name := range extract.ClusterNames {
			if name == "" {
				v.addError(path, fmt.Sprintf("cluster_names[%d]", i), "", "Cluster name cannot be empty", "EMPTY_CLUSTER_NAME", "Provide a cluster name")
				valid = false
			}
		}
	}

	// Validate array lengths match
	if len(extract.URLs) != len(extract.ClusterNames) {
		v.addError(path, "urls+cluster_names", "", "URLs and cluster_names arrays must have the same length", "MISMATCHED_ARRAYS", "Ensure each URL has a corresponding cluster name")
		valid = false
	}

	// Validate auth headers if specified
	if len(extract.AuthHeaders) > 0 && len(extract.AuthHeaders) != len(extract.URLs) {
		v.addWarning(path, "auth_headers", "", "Auth headers array length doesn't match URLs array", "MISMATCHED_AUTH_HEADERS", "Provide auth headers for all URLs or use auth_basic")
	}

	// Validate basic auth configuration
	if extract.AuthBasic != nil {
		if !v.validateBasicAuthConfig(extract.AuthBasic, path+".auth_basic", strictMode) {
			valid = false
		}
	}

	// Validate endpoint type
	if extract.EndpointType != "" {
		validTypes := []string{"generic", "urlencoded"}
		if !contains(validTypes, extract.EndpointType) {
			v.addError(path, "endpoint_type", extract.EndpointType, fmt.Sprintf("Invalid endpoint type, must be one of: %s", strings.Join(validTypes, ", ")), "INVALID_ENDPOINT_TYPE", "Use 'generic' for Elasticsearch or 'urlencoded' for Splunk")
			valid = false
		}
	}

	// Validate output format
	if extract.OutputFormat != "" {
		validFormats := []string{"json", "csv"}
		if !contains(validFormats, extract.OutputFormat) {
			v.addError(path, "output_format", extract.OutputFormat, fmt.Sprintf("Invalid output format, must be one of: %s", strings.Join(validFormats, ", ")), "INVALID_OUTPUT_FORMAT", "Use 'json' or 'csv'")
			valid = false
		}
	}

	// Validate timeout
	if extract.Timeout <= 0 {
		v.addError(path, "timeout", extract.Timeout.String(), "Timeout must be positive", "INVALID_TIMEOUT", "Set a positive duration (e.g., '30s')")
		valid = false
	} else if extract.Timeout < 5*time.Second {
		v.addWarning(path, "timeout", extract.Timeout.String(), "Very short timeout may cause frequent failures", "SHORT_TIMEOUT", "Consider using at least 5 seconds")
	}

	// Validate max retries
	if extract.MaxRetries < 0 {
		v.addError(path, "max_retries", fmt.Sprintf("%d", extract.MaxRetries), "Max retries cannot be negative", "NEGATIVE_MAX_RETRIES", "Set to 0 to disable retries or positive number")
		valid = false
	} else if extract.MaxRetries > 10 {
		v.addWarning(path, "max_retries", fmt.Sprintf("%d", extract.MaxRetries), "Very high retry count may cause long delays", "HIGH_MAX_RETRIES", "Consider using 3-5 retries")
	}

	// Validate filters
	for i, filter := range extract.Filters {
		if !v.validateFilterConfig(&filter, fmt.Sprintf("%s.filters[%d]", path, i), strictMode) {
			valid = false
		}
	}

	// Validate debug configuration
	v.validateExtractDebugConfig(&extract.Debug, path+".debug", strictMode)

	return valid
}

// validateBasicAuthConfig validates basic authentication configuration
func (v *ConfigValidator) validateBasicAuthConfig(auth *config.ExtractBasicAuthConfig, path string, strictMode bool) bool {
	valid := true

	if auth.User == "" {
		v.addError(path, "user", "", "Username is required for basic auth", "MISSING_AUTH_USER", "Provide a username")
		valid = false
	}

	if auth.Password == "" {
		v.addError(path, "password", "", "Password is required for basic auth", "MISSING_AUTH_PASSWORD", "Provide a password")
		valid = false
	}

	if auth.PasswordType != "" {
		validTypes := []string{"plain", "encrypted", "env"}
		if !contains(validTypes, auth.PasswordType) {
			v.addError(path, "password_type", auth.PasswordType, fmt.Sprintf("Invalid password type, must be one of: %s", strings.Join(validTypes, ", ")), "INVALID_PASSWORD_TYPE", "Use 'env' for environment variables")
			valid = false
		}

		if auth.PasswordType == "encrypted" && auth.Passkey == "" {
			v.addError(path, "passkey", "", "Passkey is required for encrypted password type", "MISSING_PASSKEY", "Provide encryption key")
			valid = false
		}
	}

	return valid
}

// validateFilterConfig validates filter configuration
func (v *ConfigValidator) validateFilterConfig(filter *config.FilterConfig, path string, strictMode bool) bool {
	valid := true

	validTypes := []string{"include", "exclude"}
	if !contains(validTypes, filter.Type) {
		v.addError(path, "type", filter.Type, fmt.Sprintf("Invalid filter type, must be one of: %s", strings.Join(validTypes, ", ")), "INVALID_FILTER_TYPE", "Use 'include' or 'exclude'")
		valid = false
	}

	if filter.Pattern == "" {
		v.addError(path, "pattern", "", "Filter pattern is required", "MISSING_FILTER_PATTERN", "Provide a regex pattern")
		valid = false
	} else {
		// Validate regex pattern
		if _, err := regexp.Compile(filter.Pattern); err != nil {
			v.addError(path, "pattern", filter.Pattern, fmt.Sprintf("Invalid regex pattern: %v", err), "INVALID_REGEX_PATTERN", "Use valid regex syntax")
			valid = false
		}
	}

	return valid
}

// validateExtractDebugConfig validates extract debug configuration
func (v *ConfigValidator) validateExtractDebugConfig(debug *config.ExtractDebugConfig, path string, strictMode bool) {
	if debug.Path != "" && !isValidPath(debug.Path) {
		v.addWarning(path, "path", debug.Path, "Debug path may not be accessible", "INVALID_DEBUG_PATH", "Ensure path is writable")
	}
}

// validateTransformConfig validates transform configuration
func (v *ConfigValidator) validateTransformConfig(transform *config.TransformConfig, path string, strictMode bool) bool {
	valid := true

	// Validate output format
	if transform.OutputFormat != "" {
		validFormats := []string{"json", "csv"}
		if !contains(validFormats, transform.OutputFormat) {
			v.addError(path, "output_format", transform.OutputFormat, fmt.Sprintf("Invalid output format, must be one of: %s", strings.Join(validFormats, ", ")), "INVALID_TRANSFORM_OUTPUT_FORMAT", "Use 'json' or 'csv'")
			valid = false
		}
	}

	// Validate input configuration
	if transform.Input.Format != "" {
		validFormats := []string{"json", "csv"}
		if !contains(validFormats, transform.Input.Format) {
			v.addError(path+".input", "format", transform.Input.Format, fmt.Sprintf("Invalid input format, must be one of: %s", strings.Join(validFormats, ", ")), "INVALID_INPUT_FORMAT", "Use 'json' or 'csv'")
			valid = false
		}
	}

	// Validate previous results sets
	if transform.PreviousResultsSets < 0 {
		v.addError(path, "previous_results_sets", fmt.Sprintf("%d", transform.PreviousResultsSets), "Previous results sets cannot be negative", "NEGATIVE_PREVIOUS_RESULTS", "Set to 0 for stateless or positive number")
		valid = false
	} else if transform.PreviousResultsSets > 100 {
		v.addWarning(path, "previous_results_sets", fmt.Sprintf("%d", transform.PreviousResultsSets), "Very high previous results count may consume excessive memory", "HIGH_PREVIOUS_RESULTS", "Consider using a lower value")
	}

	// Validate conversion functions
	for i, conv := range transform.ConversionFunctions {
		if !v.validateConversionFunction(&conv, fmt.Sprintf("%s.conversion_functions[%d]", path, i), strictMode) {
			valid = false
		}
	}

	// Validate debug configuration
	v.validateTransformDebugConfig(&transform.Debug, path+".debug", strictMode)

	return valid
}

// validateConversionFunction validates conversion function configuration
func (v *ConfigValidator) validateConversionFunction(conv *config.ConversionFunctionConfig, path string, strictMode bool) bool {
	valid := true

	// Either field or field_index must be specified
	if conv.Field == "" && conv.FieldIndex == 0 {
		v.addError(path, "field+field_index", "", "Either field or field_index must be specified", "MISSING_FIELD_REFERENCE", "Specify field name or column index")
		valid = false
	}

	if conv.Function == "" {
		v.addError(path, "function", "", "Conversion function is required", "MISSING_CONVERSION_FUNCTION", "Specify conversion function")
		valid = false
	} else {
		validFunctions := []string{"convert_type", "convert_to_kb", "convert_to_mb", "convert_to_gb"}
		if !contains(validFunctions, conv.Function) {
			v.addError(path, "function", conv.Function, fmt.Sprintf("Invalid conversion function, must be one of: %s", strings.Join(validFunctions, ", ")), "INVALID_CONVERSION_FUNCTION", "Use a supported conversion function")
			valid = false
		}

		// Validate function-specific parameters
		switch conv.Function {
		case "convert_type":
			if conv.FromType == "" || conv.ToType == "" {
				v.addError(path, "from_type+to_type", "", "from_type and to_type are required for convert_type function", "MISSING_TYPE_PARAMS", "Specify source and target types")
				valid = false
			} else {
				validTypes := []string{"string", "int", "float", "bool"}
				if !contains(validTypes, conv.FromType) {
					v.addError(path, "from_type", conv.FromType, fmt.Sprintf("Invalid from_type, must be one of: %s", strings.Join(validTypes, ", ")), "INVALID_FROM_TYPE", "Use a supported type")
					valid = false
				}
				if !contains(validTypes, conv.ToType) {
					v.addError(path, "to_type", conv.ToType, fmt.Sprintf("Invalid to_type, must be one of: %s", strings.Join(validTypes, ", ")), "INVALID_TO_TYPE", "Use a supported type")
					valid = false
				}
			}
		case "convert_to_kb", "convert_to_mb", "convert_to_gb":
			if conv.FromUnit != "" {
				validUnits := []string{"bytes", "b", "kb", "mb", "gb"}
				if !contains(validUnits, conv.FromUnit) {
					v.addError(path, "from_unit", conv.FromUnit, fmt.Sprintf("Invalid from_unit, must be one of: %s", strings.Join(validUnits, ", ")), "INVALID_FROM_UNIT", "Use a supported unit")
					valid = false
				}
			}
		}
	}

	return valid
}

// validateTransformDebugConfig validates transform debug configuration
func (v *ConfigValidator) validateTransformDebugConfig(debug *config.TransformDebugConfig, path string, strictMode bool) {
	if debug.Path != "" && !isValidPath(debug.Path) {
		v.addWarning(path, "path", debug.Path, "Debug path may not be accessible", "INVALID_TRANSFORM_DEBUG_PATH", "Ensure path is writable")
	}
}

// validateLoadConfig validates load configuration
func (v *ConfigValidator) validateLoadConfig(load *config.LoadConfig, path string, strictMode bool) bool {
	valid := true

	if len(load.Streams) == 0 {
		v.addError(path, "streams", "[]", "At least one load stream is required", "NO_LOAD_STREAMS", "Add at least one output stream")
		valid = false
	}

	for i, stream := range load.Streams {
		if !v.validateStreamConfig(&stream, fmt.Sprintf("%s.streams[%d]", path, i), strictMode) {
			valid = false
		}
	}

	return valid
}

// validateStreamConfig validates stream configuration
func (v *ConfigValidator) validateStreamConfig(stream *config.StreamConfig, path string, strictMode bool) bool {
	valid := true

	validTypes := []string{"prometheus", "otel", "gem", "csv", "debug"}
	if !contains(validTypes, stream.Type) {
		v.addError(path, "type", stream.Type, fmt.Sprintf("Invalid stream type, must be one of: %s", strings.Join(validTypes, ", ")), "INVALID_STREAM_TYPE", "Use a supported stream type")
		valid = false
	}

	// Validate stream-specific configuration
	switch stream.Type {
	case "prometheus", "otel", "gem":
		if endpoint, ok := stream.Config["endpoint"].(string); !ok || endpoint == "" {
			v.addError(path+".config", "endpoint", "", "Endpoint is required for "+stream.Type+" stream", "MISSING_ENDPOINT", "Provide target endpoint URL")
			valid = false
		} else if !isValidURL(endpoint) {
			v.addError(path+".config", "endpoint", endpoint, "Invalid endpoint URL format", "INVALID_ENDPOINT_URL", "Use valid URL format")
			valid = false
		}
	case "csv":
		if filepath, ok := stream.Config["filepath"].(string); !ok || filepath == "" {
			v.addError(path+".config", "filepath", "", "Filepath is required for CSV stream", "MISSING_CSV_FILEPATH", "Provide output file path")
			valid = false
		} else if !isValidPath(filepath) {
			v.addWarning(path+".config", "filepath", filepath, "CSV file path may not be accessible", "INVALID_CSV_PATH", "Ensure directory is writable")
		}
	case "debug":
		if debugPath, ok := stream.Config["path"].(string); ok && debugPath != "" {
			if !isValidPath(debugPath) {
				v.addWarning(path+".config", "path", debugPath, "Debug path may not be accessible", "INVALID_DEBUG_STREAM_PATH", "Ensure path is writable")
			}
		}
	}

	return valid
}

// Helper functions

// contains checks if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// isValidURL validates URL format
func isValidURL(urlStr string) bool {
	u, err := url.Parse(urlStr)
	return err == nil && u.Scheme != "" && u.Host != ""
}

// isValidPipelineName validates pipeline name format
func isValidPipelineName(name string) bool {
	// Allow alphanumeric characters, hyphens, and underscores
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9_-]+$`, name)
	return matched
}

// isValidTimeFormat validates time format (HH:MM:SS)
func isValidTimeFormat(timeStr string) bool {
	matched, _ := regexp.MatchString(`^([01]?[0-9]|2[0-3]):[0-5][0-9]:[0-5][0-9]$`, timeStr)
	return matched
}

// isValidPath validates if a path is potentially valid
func isValidPath(path string) bool {
	// Basic path validation - check if it's not empty and doesn't contain invalid characters
	if path == "" {
		return false
	}

	// Check for invalid characters (basic check)
	invalidChars := []string{"\x00", "<", ">", "|", "\"", "?", "*"}
	for _, char := range invalidChars {
		if strings.Contains(path, char) {
			return false
		}
	}

	return true
}

// checkDuplicatePipelineNames checks for duplicate pipeline names
func (v *ConfigValidator) checkDuplicatePipelineNames(pipelines []config.PipelineConfig) {
	nameCount := make(map[string][]int)

	for i, pipeline := range pipelines {
		if pipeline.Name != "" {
			nameCount[pipeline.Name] = append(nameCount[pipeline.Name], i)
		}
	}

	for name, indices := range nameCount {
		if len(indices) > 1 {
			for _, index := range indices {
				v.addError(fmt.Sprintf("pipelines[%d]", index), "name", name,
					fmt.Sprintf("Duplicate pipeline name '%s' found at indices: %v", name, indices),
					"DUPLICATE_PIPELINE_NAME", "Use unique pipeline names")
			}
		}
	}
}

// outputResults outputs validation results in the specified format
func outputResults(result *ValidationResult, format string, verbose bool) error {
	switch strings.ToLower(format) {
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	case "yaml":
		data, err := yaml.Marshal(result)
		if err != nil {
			return err
		}
		fmt.Print(string(data))
		return nil
	case "text":
		return outputTextResults(result, verbose)
	default:
		return fmt.Errorf("unsupported output format: %s", format)
	}
}

// outputTextResults outputs validation results in text format
func outputTextResults(result *ValidationResult, verbose bool) error {
	fmt.Printf("Configuration Validation Report\n")
	fmt.Printf("==============================\n\n")

	fmt.Printf("File: %s (%s)\n", result.ConfigPath, result.ConfigType)
	fmt.Printf("Status: ")
	if result.Valid {
		fmt.Printf("✅ VALID\n")
	} else {
		fmt.Printf("❌ INVALID\n")
	}
	fmt.Printf("\n")

	// Summary
	fmt.Printf("Summary:\n")
	fmt.Printf("  Pipelines: %d total, %d valid\n", result.Summary.PipelinesCount, result.Summary.ValidPipelines)
	fmt.Printf("  Errors: %d\n", result.Summary.TotalErrors)
	fmt.Printf("  Warnings: %d\n", result.Summary.TotalWarnings)
	if verbose {
		fmt.Printf("  Info: %d\n", result.Summary.TotalInfo)
	}
	fmt.Printf("\n")

	// Errors
	if len(result.Errors) > 0 {
		fmt.Printf("Errors:\n")
		for i, err := range result.Errors {
			fmt.Printf("  %d. [%s] %s.%s: %s\n", i+1, err.ErrorCode, err.Path, err.Field, err.Message)
			if err.Value != "" {
				fmt.Printf("     Value: %s\n", err.Value)
			}
			if err.Suggestion != "" {
				fmt.Printf("     Suggestion: %s\n", err.Suggestion)
			}
		}
		fmt.Printf("\n")
	}

	// Warnings
	if len(result.Warnings) > 0 {
		fmt.Printf("Warnings:\n")
		for i, warn := range result.Warnings {
			fmt.Printf("  %d. [%s] %s.%s: %s\n", i+1, warn.ErrorCode, warn.Path, warn.Field, warn.Message)
			if warn.Value != "" {
				fmt.Printf("     Value: %s\n", warn.Value)
			}
			if warn.Suggestion != "" {
				fmt.Printf("     Suggestion: %s\n", warn.Suggestion)
			}
		}
		fmt.Printf("\n")
	}

	// Info (only in verbose mode)
	if verbose && len(result.Info) > 0 {
		fmt.Printf("Information:\n")
		for i, info := range result.Info {
			fmt.Printf("  %d. [%s] %s.%s: %s\n", i+1, info.ErrorCode, info.Path, info.Field, info.Message)
			if info.Value != "" {
				fmt.Printf("     Value: %s\n", info.Value)
			}
			if info.Suggestion != "" {
				fmt.Printf("     Suggestion: %s\n", info.Suggestion)
			}
		}
		fmt.Printf("\n")
	}

	return nil
}

// Validation helper methods

// addError adds an error to the validation result
func (v *ConfigValidator) addError(path, field, value, message, errorCode, suggestion string) {
	v.result.Errors = append(v.result.Errors, ValidationError{
		Path:       path,
		Field:      field,
		Value:      value,
		Message:    message,
		Severity:   "error",
		Suggestion: suggestion,
		ErrorCode:  errorCode,
	})
	v.result.Valid = false
}

// addWarning adds a warning to the validation result
func (v *ConfigValidator) addWarning(path, field, value, message, errorCode, suggestion string) {
	v.result.Warnings = append(v.result.Warnings, ValidationError{
		Path:       path,
		Field:      field,
		Value:      value,
		Message:    message,
		Severity:   "warning",
		Suggestion: suggestion,
		ErrorCode:  errorCode,
	})
}

// addInfo adds an info message to the validation result
func (v *ConfigValidator) addInfo(path, field, value, message, errorCode, suggestion string) {
	v.result.Info = append(v.result.Info, ValidationError{
		Path:       path,
		Field:      field,
		Value:      value,
		Message:    message,
		Severity:   "info",
		Suggestion: suggestion,
		ErrorCode:  errorCode,
	})
}

// updateSummary updates the validation summary
func (v *ConfigValidator) updateSummary() {
	v.result.Summary.TotalErrors = len(v.result.Errors)
	v.result.Summary.TotalWarnings = len(v.result.Warnings)
	v.result.Summary.TotalInfo = len(v.result.Info)
}
