# ElasticETL - Comprehensive Technical Specification

## Table of Contents

1. [System Overview](#system-overview)
2. [Architecture](#architecture)
3. [Core Components](#core-components)
4. [Configuration Schema](#configuration-schema)
5. [Debug System](#debug-system)
6. [Authentication & Security](#authentication--security)
7. [Pipeline Resilience](#pipeline-resilience)
8. [Scheduling System](#scheduling-system)
9. [Data Processing](#data-processing)
10. [Stream Processing](#stream-processing)
11. [Monitoring & Metrics](#monitoring--metrics)
12. [Performance Specifications](#performance-specifications)
13. [API Specifications](#api-specifications)
14. [Deployment Specifications](#deployment-specifications)
15. [Error Handling](#error-handling)

## System Overview

ElasticETL is a high-performance, production-ready ETL pipeline system designed for extracting data from Elasticsearch and other compatible endpoints, transforming it through configurable processing stages, and loading it into various monitoring and analytics platforms.

### Key Characteristics

- **Language**: Go 1.21+
- **Architecture**: Multi-pipeline concurrent processing
- **Deployment**: Containerized, Kubernetes-ready
- **Configuration**: YAML-based with hot reload
- **Monitoring**: Built-in Prometheus metrics
- **Debug**: Comprehensive stage-specific debugging

## Architecture

### High-Level System Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                           ElasticETL System                         │
├─────────────────────────────────────────────────────────────────────┤
│  HTTP Server (Metrics, Health, Status)                             │
│  ├─ /metrics (Prometheus)                                          │
│  ├─ /health (Health Check)                                         │
│  ├─ /ready (Readiness Probe)                                       │
│  └─ /status (Pipeline Status)                                      │
├─────────────────────────────────────────────────────────────────────┤
│  Pipeline Manager                                                  │
│  ├─ Pipeline Lifecycle Management                                  │
│  ├─ Configuration Hot Reload                                       │
│  ├─ Resource Management                                            │
│  └─ Failure Isolation                                              │
├─────────────────────────────────────────────────────────────────────┤
│  Pipeline Instances (1..N)                                         │
│  ├─ Scheduler (Cron/Interval)                                      │
│  ├─ Extractor                                                      │
│  ├─ Transformer                                                    │
│  ├─ Loader                                                         │
│  └─ Debug System                                                   │
├─────────────────────────────────────────────────────────────────────┤
│  Shared Services                                                   │
│  ├─ Metrics Collector                                              │
│  ├─ Configuration Manager                                          │
│  ├─ Resource Monitor                                               │
│  └─ Logger                                                         │
└─────────────────────────────────────────────────────────────────────┘
```

### Component Interaction Flow

```
Configuration File
       ↓
Configuration Manager
       ↓
Pipeline Manager
       ↓
Pipeline Instances
       ↓
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Scheduler     │───▶│   Extractor     │───▶│  Transformer   │
│                 │    │                 │    │                 │
│ • Cron/Interval │    │ • HTTP Client   │    │ • Data Mapping  │
│ • Retry Logic   │    │ • Auth Handler  │    │ • Type Convert  │
│ • Failure State │    │ • JSON Parser   │    │ • CSV Format    │
└─────────────────┘    │ • Debug Output  │    │ • Debug Output  │
                       └─────────────────┘    └─────────────────┘
                                                       ↓
                       ┌─────────────────┐    ┌─────────────────┐
                       │  Debug System   │◀───│     Loader      │
                       │                 │    │                 │
                       │ • Stage Debug   │    │ • Multi-Stream  │
                       │ • File Output   │    │ • Format Conv   │
                       │ • Timestamps    │    │ • Auth Handler  │
                       └─────────────────┘    │ • Retry Logic   │
                                              └─────────────────┘
                                                       ↓
                                              ┌─────────────────┐
                                              │ Output Streams  │
                                              │                 │
                                              │ • Prometheus    │
                                              │ • OpenTelemetry │
                                              │ • GEM           │
                                              │ • CSV Files     │
                                              │ • Debug Files   │
                                              └─────────────────┘
```

## Core Components

### 1. Pipeline Manager

**Location**: `pkg/pipeline/pipeline.go`

**Responsibilities**:
- Pipeline lifecycle management (create, start, stop, update)
- Configuration hot reload
- Resource allocation and monitoring
- Failure isolation between pipelines
- Metrics collection coordination

**Key Methods**:
```go
type Manager struct {
    pipelines map[string]*Pipeline
    metrics   *metrics.Collector
    mutex     sync.RWMutex
}

func (m *Manager) AddPipeline(cfg config.PipelineConfig) error
func (m *Manager) StartAllPipelines(ctx context.Context) error
func (m *Manager) UpdatePipelines(configs []config.PipelineConfig) error
func (m *Manager) GetDetailedPipelineStatus() map[string]PipelineStatus
```

### 2. Scheduler System

**Location**: `pkg/pipeline/scheduler.go`

**Responsibilities**:
- Cron-based scheduling with expression parsing
- Interval-based scheduling
- Retry logic for failed pipelines
- Graceful shutdown handling

**Key Features**:
```go
type Scheduler struct {
    schedule     config.ScheduleConfig
    interval     time.Duration
    stopChan     chan struct{}
    goroutineDone chan struct{}
}

// Supports both cron and interval scheduling
type ScheduleConfig struct {
    CronSchedule string        `yaml:"cron_schedule"`
    Interval     time.Duration `yaml:"interval"`
}
```

### 3. Extractor

**Location**: `pkg/extract/extractor.go`

**Responsibilities**:
- HTTP client management with connection pooling
- Authentication handling (Bearer, Basic, Custom headers)
- JSON path extraction and data flattening
- Regex-based filtering
- CSV data parsing
- Debug output generation

**Key Features**:
- Multiple endpoint support with failover
- Environment variable substitution
- TLS configuration
- Retry logic with exponential backoff
- Endpoint type support (Elasticsearch, Splunk)

**Debug Configuration**:
```go
type ExtractDebugConfig struct {
    Path        string `yaml:"path"`
    FinalQuery  bool   `yaml:"final_query"`
    APIResponse bool   `yaml:"api_response"`
    FinalOutput bool   `yaml:"final_output"`
}
```

### 4. Transformer

**Location**: `pkg/transform/transformer.go`

**Responsibilities**:
- Data type conversions
- Unit conversions (bytes to KB/MB/GB)
- Null value handling
- CSV format conversion with advanced flattening
- Regex-based field processing
- Debug output generation

**Key Features**:
- Stateless and stateful processing modes
- Advanced JSON flattening with depth-based key analysis
- Dynamic state tracking for CSV generation
- Previous results retention
- Comprehensive conversion functions

**Debug Configuration**:
```go
type TransformDebugConfig struct {
    Path              string `yaml:"path"`
    Input             bool   `yaml:"input"`
    TransformedOutput bool   `yaml:"transformed_output"`
    FinalOutput       bool   `yaml:"final_output"`
}
```

### 5. Loader

**Location**: `pkg/load/loader.go`

**Responsibilities**:
- Multi-stream output processing
- Format-specific serialization
- Authentication for output endpoints
- Concurrent loading with error handling
- CSV-based time series generation

**Supported Stream Types**:
- Prometheus (pushgateway/remote write)
- OpenTelemetry (OTEL collector)
- GEM (Prometheus remote write)
- CSV files
- Debug output (JSON/Prometheus/OTEL formats)

## Configuration Schema

### Complete Configuration Structure

```yaml
# Global configuration
global:
  resource_limits:
    max_memory_mb: int              # Memory limit in MB
    max_cpu_percent: int            # CPU usage limit
    max_goroutines: int             # Goroutine limit
  metrics:
    enabled: boolean                # Enable metrics server
    port: int                       # Metrics server port
  logging:
    level: string                   # debug, info, warn, error
    format: string                  # json, text
    output: string                  # stdout, stderr, file path

# Pipeline configurations
pipelines:
  - name: string                    # Unique pipeline identifier
    enabled: boolean                # Enable/disable pipeline
    interval: duration              # Execution interval
    retryInterval: duration         # Retry interval for failures
    
    # Alternative scheduling
    schedule:
      cron_schedule: string         # Cron expression
      interval: duration            # Fallback interval
    
    # Extract configuration
    extract:
      elasticsearch_query: string   # DSL query
      urls: []string                # Endpoint URLs
      cluster_names: []string       # Cluster identifiers
      endpoint_type: string         # "generic" or "urlencoded"
      
      # Authentication
      auth_headers: []string        # Bearer tokens
      auth_basic:                   # Basic authentication
        user: string
        password: string
        password_type: string       # "plain", "encrypted", "env"
        passkey: string             # Encryption key
      additional_headers: [][]string # Custom headers
      
      # Data processing
      json_path: string             # JSON extraction path
      output_format: string         # "json" or "csv"
      filters:                      # Field filters
        - type: string              # "include" or "exclude"
          pattern: string           # Regex pattern
      
      # Connection settings
      timeout: duration             # Request timeout
      max_retries: int              # Retry attempts
      insecure_tls: boolean         # Skip TLS verification
      
      # Debug configuration
      debug:
        path: string                # Debug output directory
        final_query: boolean        # Debug processed query
        api_response: boolean       # Debug API responses
        final_output: boolean       # Debug extracted data
    
    # Transform configuration
    transform:
      stateless: boolean            # Stateless processing
      substitute_zeros_for_null: boolean # Null handling
      drop_null_values: boolean     # Drop null values
      previous_results_sets: int    # History retention
      output_format: string         # "json" or "csv"
      
      # Input format handling
      input:
        format: string              # "json" or "csv"
        header: boolean             # CSV has header row
      
      # Data conversions
      conversion_functions:
        - field: string             # Field pattern (regex)
          field_index: int          # CSV column index
          function: string          # Conversion function
          from_type: string         # Source type
          to_type: string           # Target type
          from_unit: string         # Source unit
      
      # Debug configuration
      debug:
        path: string                # Debug output directory
        input: boolean              # Debug input data
        transformed_output: boolean # Debug transformed data
        final_output: boolean       # Debug final output
    
    # Load configuration
    load:
      streams:
        - type: string              # Stream type
          config:                   # Stream-specific config
            endpoint: string        # Target endpoint
            path: string            # File path
            format: string          # Output format
            timeout: duration       # Request timeout
            metrics:                # CSV-based metrics
              - name: string        # Metric name
                uniquefieldsIndex: []int # Grouping columns
                value: int          # Value column
                timestamp: int      # Timestamp column
                labels:             # Label configuration
                  - label_name: string
                    index_in_csv_data: int
                  - label_name: string
                    static_value: string
          basic_auth:               # Basic authentication
            username: string
            password: string
          insecure_tls: boolean     # Skip TLS verification
          labels:                   # Static labels
            key: value
```

## Debug System

### Architecture

The debug system provides comprehensive visibility into pipeline execution with stage-specific output and granular control over captured information.

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Debug System                                │
├─────────────────────────────────────────────────────────────────────┤
│  Pipeline-Level Debug Coordination                                 │
│  ├─ shouldWriteExtractDebugOutput()                                │
│  ├─ writeExtractDebugOutput()                                      │
│  ├─ shouldWriteTransformDebugOutput()                              │
│  └─ writeTransformDebugOutput()                                    │
├─────────────────────────────────────────────────────────────────────┤
│  Extract Stage Debug                                               │
│  ├─ Final Query Debug (processed DSL query)                       │
│  ├─ API Response Debug (response metadata)                        │
│  └─ Final Output Debug (extracted data)                           │
├─────────────────────────────────────────────────────────────────────┤
│  Transform Stage Debug                                             │
│  ├─ Input Debug (data from extract stage)                         │
│  ├─ Transformed Output Debug (after conversions)                  │
│  └─ Final Output Debug (final processed data)                     │
├─────────────────────────────────────────────────────────────────────┤
│  Debug File Management                                             │
│  ├─ Pipeline-Specific Filenames                                   │
│  ├─ Timestamp-Based Organization                                  │
│  ├─ Configurable Output Directories                               │
│  └─ JSON Format Output                                            │
└─────────────────────────────────────────────────────────────────────┘
```

### Debug File Naming Convention

```
Format: {pipeline_name}_{stage}_{timestamp}.json
Examples:
- basic-metrics_extract_20231027_131530.json
- data-pipeline_transform_20231027_131531.json
- monitoring_extract_20231027_131532.json
```

### Debug Output Structure

#### Extract Debug Output
```json
{
  "timestamp": "2023-10-27T13:15:30Z",
  "stage": "extract",
  "pipeline_name": "basic-metrics",
  "final_query": "{ \"query\": { \"match_all\": {} } }",
  "api_responses": [
    {
      "source": "http://localhost:9200/logs-*/_search",
      "timestamp": "2023-10-27T13:15:30Z",
      "metadata": {
        "endpoint": "http://localhost:9200/logs-*/_search",
        "cluster_name": "local",
        "response_size": 1024
      }
    }
  ],
  "final_output": [
    {
      "source": "http://localhost:9200/logs-*/_search",
      "timestamp": "2023-10-27T13:15:30Z",
      "data": {
        "aggregations.doc_count.value": 150
      }
    }
  ]
}
```

#### Transform Debug Output
```json
{
  "timestamp": "2023-10-27T13:15:31Z",
  "stage": "transform",
  "pipeline_name": "basic-metrics",
  "input": [
    {
      "source": "http://localhost:9200/logs-*/_search",
      "timestamp": "2023-10-27T13:15:30Z",
      "data": {
        "aggregations.doc_count.value": 150
      }
    }
  ],
  "transformed_output": [
    {
      "source": "http://localhost:9200/logs-*/_search",
      "timestamp": "2023-10-27T13:15:30Z",
      "transformed_data": {
        "doc_count": 150.0
      }
    }
  ],
  "final_output": [
    {
      "source": "http://localhost:9200/logs-*/_search",
      "timestamp": "2023-10-27T13:15:30Z",
      "csv_data": [
        ["doc_count"],
        ["150.0"]
      ],
      "csv_headers": ["doc_count"]
    }
  ]
}
```

## Authentication & Security

### Authentication Methods

#### 1. Bearer Token Authentication
```go
// Environment variable substitution
authHeader := substituteEnvVars(e.config.AuthHeaders[index])
req.Header.Set("Authorization", authHeader)
```

#### 2. Basic Authentication
```go
type BasicAuthConfig struct {
    User         string `yaml:"user"`
    Password     string `yaml:"password"`
    PasswordType string `yaml:"password_type"` // "plain", "encrypted", "env"
    Passkey      string `yaml:"passkey"`
}
```

#### 3. Password Encryption
```go
// Utility functions in pkg/utils
func ProcessBasicAuthPassword(password, passwordType, passkey string) (string, error)
func CreateBasicAuthHeader(user, password string) string
```

### Environment Variable Substitution

**Pattern**: `${VARIABLE_NAME}`

**Implementation**:
```go
func substituteEnvVars(input string) string {
    re := regexp.MustCompile(`\$\{([^}]+)\}`)
    return re.ReplaceAllStringFunc(input, func(match string) string {
        varName := strings.TrimPrefix(strings.TrimSuffix(match, "}"), "${")
        if envValue := os.Getenv(varName); envValue != "" {
            return envValue
        }
        return match
    })
}
```

### TLS Configuration

```go
transport := &http.Transport{}
if cfg.InsecureTLS {
    transport.TLSClientConfig = &tls.Config{
        InsecureSkipVerify: true,
    }
}
```

## Pipeline Resilience

### Failure States and Transitions

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   RUNNING +     │───▶│   RUNNING +     │───▶│   RUNNING +     │
│   SUCCESSFUL    │    │     FAILED      │    │   SUCCESSFUL    │
│                 │    │                 │    │                 │
│ • Normal exec   │    │ • Skip exec     │    │ • Normal exec   │
│ • Clear failure │    │ • Wait retry    │    │ • Clear failure │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         ▲                       │                       ▲
         │                       │                       │
         │              ┌─────────────────┐              │
         │              │   RETRY TIME    │              │
         │              │    REACHED      │              │
         │              │                 │              │
         │              │ • Attempt exec  │              │
         │              │ • Update state  │              │
         │              └─────────────────┘              │
         │                       │                       │
         └───────────────────────┼───────────────────────┘
                                 │
                        ┌─────────────────┐
                        │   EXECUTION     │
                        │    RESULT       │
                        │                 │
                        │ • Success: →    │
                        │ • Failure: ↑    │
                        └─────────────────┘
```

### Retry Logic Implementation

```go
type Pipeline struct {
    failed          bool
    lastFailureTime time.Time
    config          config.PipelineConfig
}

func (p *Pipeline) execute(ctx context.Context) {
    // Check if pipeline is in failed state
    if p.IsFailed() {
        if time.Since(p.GetLastFailureTime()) < p.config.RetryInterval {
            return // Skip execution until retry interval passes
        }
    }
    
    // Execute pipeline stages
    if err := p.runETLStages(ctx); err != nil {
        p.markAsFailed()
        return
    }
    
    p.markAsSuccessful()
}
```

### Error Isolation

```go
func (p *Pipeline) execute(ctx context.Context) {
    defer func() {
        if r := recover(); r != nil {
            p.markAsFailed()
            p.metrics.RecordPipelineFailure(p.config.Name, 
                time.Since(startTime), 
                fmt.Errorf("pipeline panic: %v", r))
        }
    }()
    // Pipeline execution logic
}
```

## Scheduling System

### Cron Expression Support

**Supported Format**: `second minute hour day month weekday`

**Examples**:
- `0 */5 * * * *` - Every 5 minutes
- `0 0 */2 * * *` - Every 2 hours
- `0 30 9 * * 1-5` - 9:30 AM on weekdays

### Implementation

```go
type Scheduler struct {
    schedule      config.ScheduleConfig
    interval      time.Duration
    cronSchedule  cron.Schedule
    stopChan      chan struct{}
    goroutineDone chan struct{}
}

func (s *Scheduler) Start(ctx context.Context, executeFunc func()) error {
    if s.schedule.CronSchedule != "" {
        return s.startCronScheduler(ctx, executeFunc)
    }
    return s.startIntervalScheduler(ctx, executeFunc)
}
```

### Graceful Shutdown

```go
func (s *Scheduler) Stop() {
    s.stopOnce.Do(func() {
        close(s.stopChan)
        // Wait for goroutine to finish with timeout
        select {
        case <-s.goroutineDone:
        case <-time.After(5 * time.Second):
            // Force shutdown after timeout
        }
    })
}
```

## Data Processing

### JSON Flattening Algorithm

The transformer implements sophisticated JSON flattening with depth-based key analysis:

```go
func (t *Transformer) flattenJSON(data interface{}, prefix string) map[string]interface{} {
    result := make(map[string]interface{})
    
    switch v := data.(type) {
    case map[string]interface{}:
        // Handle single key-value pair with "value" key (case insensitive)
        if len(v) == 1 {
            for key, value := range v {
                if strings.ToLower(key) == "value" {
                    if prefix != "" {
                        result[prefix] = value
                    } else {
                        result["value"] = value
                    }
                    return result
                }
            }
        }
        
        // Regular object flattening
        for key, value := range v {
            newKey := key
            if prefix != "" {
                newKey = prefix + "." + key
            }
            flattened := t.flattenJSON(value, newKey)
            for k, v := range flattened {
                result[k] = v
            }
        }
        
    case []interface{}:
        // Handle arrays - create indexed keys
        for i, item := range v {
            indexKey := fmt.Sprintf("%s[%d]", prefix, i)
            if prefix == "" {
                indexKey = fmt.Sprintf("[%d]", i)
            }
            flattened := t.flattenJSON(item, indexKey)
            for k, v := range flattened {
                result[k] = v
            }
        }
        
    default:
        // Primitive value
        if prefix != "" {
            result[prefix] = v
        } else {
            result["value"] = v
        }
    }
    
    return result
}
```

### CSV Generation with Dynamic State Tracking

The system implements advanced CSV generation using dynamic state tracking:

```go
func (t *Transformer) generateCSVRows(data map[string]interface{}, uniqueKeys []string) [][]string {
    // Initialize state tracking maps
    valueMap := make(map[string]string, len(uniqueKeys))
    boolMap := make(map[string]bool, len(uniqueKeys))
    depthMap := make(map[string]int, len(uniqueKeys))
    
    // Process sorted flattened data
    for _, key := range sortedKeys {
        uniqueKey := t.removeArrayIndices(key)
        keyDepth := depthMap[uniqueKey]
        value := t.formatValue(data[key])
        
        // Handle depth changes
        if keyDepth < currentDepth {
            // Reset deeper levels when depth decreases
            for uk := range depthMap {
                if depthMap[uk] >= keyDepth && uk != uniqueKey {
                    valueMap[uk] = ""
                    boolMap[uk] = false
                }
            }
        }
        
        // Update state
        valueMap[uniqueKey] = value
        boolMap[uniqueKey] = true
        
        // Check if all booleans are true - create CSV row
        if allTrue(boolMap) {
            row := make([]string, len(uniqueKeys))
            for i, uniqueKey := range uniqueKeys {
                row[i] = valueMap[uniqueKey]
            }
            csvRows = append(csvRows, row)
            
            // Reset maximum depth entries
            resetMaxDepthEntries(valueMap, boolMap, depthMap)
        }
    }
    
    return csvRows
}
```

### Type Conversion System

```go
type ConversionFunctionConfig struct {
    Field      string `yaml:"field"`       // Regex pattern for field matching
    FieldIndex int    `yaml:"field_index"` // CSV column index
    Function   string `yaml:"function"`    // Conversion function name
    FromType   string `yaml:"from_type"`   // Source data type
    ToType     string `yaml:"to_type"`     // Target data type
    FromUnit   string `yaml:"from_unit"`   // Source unit for conversions
}

// Supported conversion functions
func (t *Transformer) convertType(value interface{}, fromType, toType string) (interface{}, error)
func (t *Transformer) convertToKB(value interface{}, fromUnit string) (float64, error)
func (t *Transformer) convertToMB(value interface{}, fromUnit string) (float64, error)
func (t *Transformer) convertToGB(value interface{}, fromUnit string) (float64, error)
```

## Stream Processing

### Stream Type Architecture

```go
type StreamConfig struct {
    Type         string                 `yaml:"type"`
    Config       map[string]interface{} `yaml:"config"`
    BasicAuth    *BasicAuthConfig       `yaml:"basic_auth,omitempty"`
    InsecureTLS  bool                   `yaml:"insecure_tls,omitempty"`
    Labels       map[string]string      `yaml:"labels,omitempty"`
}
```

### Prometheus Time Series Generation

```go
type MetricConfig struct {
    Name               string        `yaml:"name"`
    UniqueFieldsIndex  []int         `yaml:"uniquefieldsIndex"`
    Value              int           `yaml:"value"`
    Timestamp          int           `yaml:"timestamp"`
    Labels             []LabelConfig `yaml:"labels"`
}

type LabelConfig struct {
    LabelName        string `yaml:"label_name"`
    IndexInCSVData   int    `yaml:"index_in_csv_data,omitempty"`
    StaticValue      string `yaml:"static_value,omitempty"`
}
```

### Stream Processing Flow

```
CSV Data Input
       ↓
Group by UniqueFieldsIndex
       ↓
For each unique group:
  ├─ Extract timestamp from specified column
  ├─ Extract value from specified column
  ├─ Generate labels (static + dynamic)
  └─ Create time series entry
       ↓
Format for target system:
  ├─ Prometheus: Remote write format
  ├─ OTEL: OTEL metrics format
  ├─ Debug: Human-readable format
  └─ CSV: Structured CSV output
```

## Monitoring & Metrics

### Built-in Prometheus Metrics

```go
// Pipeline execution metrics
elasticetl_pipeline_executions_total{pipeline="name", status="success|failure"}
elasticetl_pipeline_duration_seconds{pipeline="name"}
elasticetl_pipeline_errors_total{pipeline="name", error_type="extract|transform|load"}

// Extract phase metrics
elasticetl_extract_requests_total{pipeline="name", endpoint="url"}
elasticetl_extract_duration_seconds{pipeline="name", endpoint="url"}

// Transform phase metrics
elasticetl_transform_records_total{pipeline="name"}
elasticetl_transform_duration_seconds{pipeline="name"}

// Load phase metrics
elasticetl_load_requests_total{pipeline="name", stream_type="prometheus|otel|csv|debug"}
elasticetl_load_duration_seconds{pipeline="name", stream_type="type"}

// System metrics
elasticetl_memory_usage_bytes
elasticetl_goroutines_active
elasticetl_cpu_usage_percent
```

### Health Check Implementation

```go
func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
    status := map[string]interface{}{
        "status":    "healthy",
        "timestamp": time.Now().Format(time.RFC3339),
        "version":   version,
        "uptime":    time.Since(s.startTime).String(),
    }
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(status)
}

func (s *Server) readyHandler(w http.ResponseWriter, r *http.Request) {
    // Check if all critical components are ready
    if !s.pipelineManager.IsReady() {
        w.WriteHeader(http.StatusServiceUnavailable)
        return
    }
    
    w.WriteHeader(http.StatusOK)
    w.Write([]byte("ready"))
}
```

## Performance Specifications

### System Requirements

| Component | Minimum | Recommended | Maximum |
|-----------|---------|-------------|---------|
| Memory | 128MB | 512MB | 2GB |
| CPU | 1 Core @ 1GHz | 2 Cores @ 2GHz | 8 Cores @ 3GHz |
| Disk Space | 100MB | 1GB | 10GB |
| Network | 1Mbps | 10Mbps | 100Mbps |
| File Descriptors | 1024 | 4096 | 65536 |

### Performance Characteristics

| Metric | Value | Conditions |
|--------|-------|------------|
| Max Concurrent Pipelines | 100+ | With adequate resources |
| Pipeline Execution Frequency | 1s minimum | Configurable interval |
| Data Throughput | 10MB/s | Per pipeline |
| Query Response Time | <5s | 95th percentile |
| Memory Usage | <512MB | Typical workload |
| CPU Usage | <50% | Typical workload |
| Goroutine Count | <1000 | Normal operation |

### Resource Management

```go
type ResourceLimits struct {
    MaxMemoryMB    int `yaml:"max_memory_mb"`
    MaxCPUPercent  int `yaml:"max_cpu_percent"`
    MaxGoroutines  int
