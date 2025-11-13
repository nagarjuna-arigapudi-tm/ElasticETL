# ElasticETL - Comprehensive Documentation

A high-performance, production-ready ETL (Extract, Transform, Load) pipeline for processing Elasticsearch data and delivering it to various monitoring and analytics platforms.

## Table of Contents

1. [Overview](#overview)
2. [Features](#features)
3. [Quick Start](#quick-start)
4. [Architecture](#architecture)
5. [Configuration](#configuration)
6. [Debug Capabilities](#debug-capabilities)
7. [Authentication & Security](#authentication--security)
8. [Pipeline Resilience](#pipeline-resilience)
9. [Monitoring & Metrics](#monitoring--metrics)
10. [Stream Types](#stream-types)
11. [Data Transformation](#data-transformation)
12. [Deployment](#deployment)
13. [Performance](#performance)
14. [Troubleshooting](#troubleshooting)
15. [API Reference](#api-reference)

## Overview

ElasticETL is a sophisticated ETL pipeline designed specifically for extracting data from Elasticsearch, transforming it into various formats, and loading it into multiple monitoring and analytics platforms. It provides enterprise-grade features including multi-pipeline support, comprehensive debugging, authentication, resilience mechanisms, and extensive monitoring capabilities.

## Features

### Core Features
- **Multi-Pipeline Support**: Run multiple ETL pipelines concurrently with complete isolation
- **Flexible Data Extraction**: Query Elasticsearch with custom DSL queries and JSON path extraction
- **Advanced Transformations**: Type conversions, regex filtering, CSV formatting, and data flattening
- **Multiple Output Streams**: Prometheus, OpenTelemetry, GEM, CSV, and debug outputs
- **Hot Configuration Reload**: Update configurations without restart
- **Resource Management**: Configurable memory, CPU, and connection limits

### Security & Authentication
- **Multiple Auth Methods**: Bearer tokens, basic auth, API keys, custom headers
- **Environment Variables**: Secure credential management with `${VAR_NAME}` substitution
- **TLS Support**: Configurable TLS settings with insecure mode option
- **Password Encryption**: Built-in password encryption utilities

### Resilience & Reliability
- **Pipeline Isolation**: Failed pipelines don't affect others
- **Retry Mechanism**: Configurable retry intervals for failed pipelines
- **Circuit Breaker**: Automatic failure detection and recovery
- **Graceful Error Handling**: Panic recovery and proper error isolation

### Debug & Monitoring
- **Comprehensive Debug Support**: Stage-specific debug output with granular control
- **Pipeline-Specific Debug Files**: Organized debug output with timestamps
- **Built-in Prometheus Metrics**: Detailed performance and health metrics
- **Health Checks**: Ready and liveness probes
- **Structured Logging**: JSON and text format logging

### Advanced Features
- **Cron Scheduling**: Support for cron expressions and interval-based scheduling
- **CSV Time Series**: Generate Prometheus time series from CSV data
- **Dynamic Labels**: Create metrics with dynamic labels from data
- **Endpoint Types**: Support for different API endpoint types (Elasticsearch, Splunk)
- **Data Flattening**: Advanced JSON flattening with depth-based key analysis

## Quick Start

### 1. Installation

```bash
# Clone the repository
git clone <repository-url>
cd ElasticETL

# Build the application
make build

# Or run directly
go run ./cmd/elasticetl
```

### 2. Basic Configuration

Create a `config.yaml` file:

```yaml
pipelines:
  - name: "basic-metrics"
    enabled: true
    interval: "60s"
    retryInterval: "1h"  # Retry failed pipelines after 1 hour
    
    extract:
      elasticsearch_query: |
        {
          "query": {"match_all": {}},
          "aggs": {
            "doc_count": {"value_count": {"field": "_id"}}
          }
        }
      urls:
        - "http://localhost:9200/logs-*/_search"
      cluster_names:
        - "local"
      json_path: "aggregations"
      timeout: "30s"
      max_retries: 3
      debug:
        path: "debug"
        final_query: true
        api_response: true
        final_output: true
    
    transform:
      stateless: true
      substitute_zeros_for_null: true
      output_format: "csv"
      debug:
        path: "debug"
        input: true
        transformed_output: true
        final_output: true
    
    load:
      streams:
        - type: "debug"
          config:
            path: "/tmp/elasticetl/output"
            format: "json"

global:
  resource_limits:
    max_memory_mb: 256
    max_cpu_percent: 50
  metrics:
    enabled: true
    port: 8080
  logging:
    level: "info"
    format: "json"
    output: "stdout"
```

### 3. Run ElasticETL

```bash
./elasticetl --config config.yaml
```

## Architecture

### High-Level Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│    EXTRACT      │───▶│   TRANSFORM     │───▶│      LOAD       │
│                 │    │                 │    │                 │
│ • Elasticsearch │    │ • Data Mapping  │    │ • Prometheus    │
│ • Splunk        │    │ • Type Convert  │    │ • OpenTelemetry │
│ • JSON Path     │    │ • CSV Format    │    │ • GEM           │
│ • Filtering     │    │ • Regex Match   │    │ • CSV Files     │
│ • Auth/TLS      │    │ • Null Handling │    │ • Debug Output  │
│ • Retry Logic   │    │ • Data Flatten  │    │ • Time Series   │
└─────────────────┘    └─────────────────┘    └─────────────────┘
```

### Component Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│                        ElasticETL Core                           │
├──────────────────────────────────────────────────────────────────┤
│  Pipeline Manager                                                │
│  ├─ Pipeline 1 (Extract → Transform → Load)                     │
│  ├─ Pipeline 2 (Extract → Transform → Load)                     │
│  └─ Pipeline N (Extract → Transform → Load)                     │
├──────────────────────────────────────────────────────────────────┤
│  Debug System                                                   │
│  ├─ Extract Debug (final_query, api_response, final_output)     │
│  ├─ Transform Debug (input, transformed_output, final_output)   │
│  └─ Pipeline-Specific Debug Files                               │
├──────────────────────────────────────────────────────────────────┤
│  Scheduler System                                               │
│  ├─ Cron Scheduling                                             │
│  ├─ Interval Scheduling                                         │
│  └─ Retry Logic                                                 │
├──────────────────────────────────────────────────────────────────┤
│  Resource Manager                                               │
│  ├─ Memory Limits                                               │
│  ├─ CPU Limits                                                  │
│  ├─ Connection Pooling                                          │
│  └─ Goroutine Management                                        │
├──────────────────────────────────────────────────────────────────┤
│  Configuration Manager                                           │
│  ├─ Hot Reload                                                  │
│  ├─ Environment Variables                                       │
│  └─ Validation                                                  │
├──────────────────────────────────────────────────────────────────┤
│  Metrics & Monitoring                                           │
│  ├─ Prometheus Metrics                                          │
│  ├─ Health Checks                                               │
│  └─ Performance Monitoring                                      │
└──────────────────────────────────────────────────────────────────┘
```

## Configuration

### Pipeline Configuration Schema

```yaml
pipelines:
  - name: string                    # Pipeline identifier
    enabled: boolean                # Enable/disable pipeline
    interval: duration              # Execution interval (e.g., "30s", "5m")
    retryInterval: duration         # Retry interval for failed pipelines
    schedule:                       # Alternative to interval
      cron_schedule: string         # Cron expression (e.g., "0 */5 * * * *")
      interval: duration            # Fallback interval
    
    extract:
      elasticsearch_query: string   # Elasticsearch DSL query
      urls: []string                # Elasticsearch endpoints
      cluster_names: []string       # Cluster identifiers
      endpoint_type: string         # "generic" (default) or "urlencoded"
      auth_headers: []string        # Authentication headers
      auth_basic:                   # Basic authentication
        user: string
        password: string
        password_type: string       # "plain", "encrypted", "env"
        passkey: string             # For encrypted passwords
      additional_headers: [][]string # Custom headers
      json_path: string             # JSON extraction path
      filters: []FilterConfig       # Field filters
      timeout: duration             # Request timeout
      max_retries: int              # Retry attempts
      insecure_tls: boolean         # Skip TLS verification
      output_format: string         # "json" (default) or "csv"
      debug:                        # Extract debug configuration
        path: string                # Debug output directory
        final_query: boolean        # Debug final query
        api_response: boolean       # Debug API response
        final_output: boolean       # Debug final output
    
    transform:
      stateless: boolean            # Stateless processing
      substitute_zeros_for_null: boolean # Null handling
      drop_null_values: boolean     # Drop null values
      previous_results_sets: int    # History retention
      output_format: string         # "json" or "csv"
      input:                        # Input format configuration
        format: string              # "json" or "csv"
        header: boolean             # CSV has header row
      conversion_functions: []ConversionFunctionConfig
      debug:                        # Transform debug configuration
        path: string                # Debug output directory
        input: boolean              # Debug input data
        transformed_output: boolean # Debug transformed data
        final_output: boolean       # Debug final output
    
    load:
      streams: []StreamConfig       # Output destinations
```

### Stream Configuration

#### Prometheus Stream
```yaml
- type: "prometheus"
  config:
    endpoint: string                # Pushgateway or remote write URL
    timeout: duration               # Request timeout
    metrics:                        # CSV-based time series
      - name: string                # Metric name
        uniquefieldsIndex: []int    # Grouping columns
        value: int                  # Value column index
        timestamp: int              # Timestamp column index
        labels:                     # Label configuration
          - label_name: string
            index_in_csv_data: int  # Dynamic label from CSV
          - label_name: string
            static_value: string    # Static label value
  basic_auth:
    username: string                # Basic auth username
    password: string                # Basic auth password
  insecure_tls: boolean            # Skip TLS verification
  labels:                          # Static labels
    key: value
```

#### Debug Stream
```yaml
- type: "debug"
  config:
    path: string                    # Output file path
    format: string                  # "json", "prometheus", "otel"
    metrics: []MetricConfig         # Same as Prometheus metrics
```

#### OpenTelemetry Stream
```yaml
- type: "otel"
  config:
    endpoint: string                # OTEL collector endpoint
    timeout: duration               # Request timeout
  insecure_tls: boolean            # Skip TLS verification
  labels:                          # Resource attributes
    key: value
```

#### CSV Stream
```yaml
- type: "csv"
  config:
    path: string                    # Output file path
```

#### GEM Stream
```yaml
- type: "gem"
  config:
    endpoint: string                # GEM endpoint
    timeout: duration               # Request timeout
    metrics: []MetricConfig         # CSV-based time series
  insecure_tls: boolean            # Skip TLS verification
  labels:                          # Static labels
    key: value
```

## Debug Capabilities

ElasticETL provides comprehensive debugging capabilities with stage-specific debug output and granular control over what information is captured.

### Extract Stage Debug

Configure extract debug options to capture query execution and API response details:

```yaml
extract:
  debug:
    path: "debug"                   # Debug output directory (default: "debug")
    final_query: true               # Debug the final processed query
    api_response: true              # Debug API response metadata
    final_output: true              # Debug final extracted data
```

**Debug Output**: `{pipeline_name}_extract_{timestamp}.json`

**Example Output**:
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
      "response_size": 1024
    }
  ],
  "final_output": [
    {
      "source": "http://localhost:9200/logs-*/_search",
      "data": { "aggregations": { "doc_count": { "value": 150 } } }
    }
  ]
}
```

### Transform Stage Debug

Configure transform debug options to capture input data, transformation process, and final output:

```yaml
transform:
  debug:
    path: "debug"                   # Debug output directory (default: "debug")
    input: true                     # Debug input data from extract stage
    transformed_output: true        # Debug data after transformations
    final_output: true              # Debug final output data
```

**Debug Output**: `{pipeline_name}_transform_{timestamp}.json`

**Example Output**:
```json
{
  "timestamp": "2023-10-27T13:15:31Z",
  "stage": "transform",
  "pipeline_name": "basic-metrics",
  "input": [
    {
      "source": "http://localhost:9200/logs-*/_search",
      "data": { "aggregations.doc_count.value": 150 }
    }
  ],
  "transformed_output": [
    {
      "source": "http://localhost:9200/logs-*/_search",
      "transformed_data": { "doc_count": 150.0 }
    }
  ],
  "final_output": [
    {
      "source": "http://localhost:9200/logs-*/_search",
      "csv_data": [["doc_count"], ["150.0"]],
      "csv_headers": ["doc_count"]
    }
  ]
}
```

### Debug Stream Formats

Debug streams support multiple output formats for different use cases:

#### JSON Format (Default)
```yaml
- type: "debug"
  config:
    path: "/tmp/debug/json-output"
    format: "json"
```

#### Prometheus Format
```yaml
- type: "debug"
  config:
    path: "/tmp/debug/prometheus-output"
    format: "prometheus"
```

#### OpenTelemetry Format
```yaml
- type: "debug"
  config:
    path: "/tmp/debug/otel-output"
    format: "otel"
```

### Debug Best Practices

1. **Development**: Enable all debug options during development
2. **Production**: Use selective debug options to avoid performance impact
3. **Troubleshooting**: Enable debug streams with specific formats to diagnose issues
4. **Storage**: Monitor debug file sizes and implement rotation if needed

## Authentication & Security

### Authentication Methods

#### Bearer Token Authentication
```yaml
auth_headers:
  - "Bearer ${ES_TOKEN}"
```

#### Basic Authentication
```yaml
auth_basic:
  user: "${ES_USERNAME}"
  password: "${ES_PASSWORD}"
  password_type: "env"  # "plain", "encrypted", "env"
```

#### Encrypted Password Support
```yaml
auth_basic:
  user: "elasticsearch"
  password: "encrypted_password_here"
  password_type: "encrypted"
  passkey: "${ENCRYPTION_KEY}"
```

#### Custom Headers
```yaml
additional_headers:
  - ["X-API-Key: ${API_KEY}", "X-Environment: production"]
```

### Environment Variable Substitution

Pattern: `${VARIABLE_NAME}`

Supported in:
- Authentication headers
- Basic auth credentials
- Custom headers
- Endpoint URLs
- Any string configuration value

### TLS Configuration

```yaml
insecure_tls: false  # Default: validate certificates
timeout: "30s"       # Connection timeout
```

### Password Encryption Utility

ElasticETL includes a password encryption utility:

```bash
# Encrypt a password
./encrypt-password --password "mypassword" --key "encryption-key"

# Use in configuration
auth_basic:
  user: "username"
  password: "encrypted_output_here"
  password_type: "encrypted"
  passkey: "${ENCRYPTION_KEY}"
```

## Pipeline Resilience

### Retry Mechanism

Configure automatic retry for failed pipelines:

```yaml
pipelines:
  - name: "resilient-pipeline"
    enabled: true
    interval: "60s"
    retryInterval: "1h"  # Retry after 1 hour of failure
```

**Retry Behavior**:
1. Pipeline fails (extract, transform, or load error)
2. Pipeline is marked as failed with timestamp
3. Normal execution is skipped until retry interval passes
4. After retry interval, pipeline attempts execution again
5. Success clears failure state; failure updates failure timestamp

### Pipeline Isolation

- Each pipeline runs in complete isolation
- Panics in one pipeline don't affect others
- Failures don't cascade between pipelines
- Independent failure states and retry logic

### Error Handling

```yaml
extract:
  max_retries: 3        # Retry failed requests
  timeout: "30s"        # Request timeout
  
load:
  streams:
    - type: "prometheus"
      config:
        timeout: "10s"  # Stream-specific timeout
```

### Failure States

Pipeline states:
- **Running + Successful**: Normal operation
- **Running + Failed**: Skipping execution until retry
- **Stopped**: Pipeline disabled

## Monitoring & Metrics

### Built-in Prometheus Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `elasticetl_pipeline_executions_total` | Counter | Total pipeline executions |
| `elasticetl_pipeline_duration_seconds` | Histogram | Pipeline execution time |
| `elasticetl_pipeline_errors_total` | Counter | Pipeline errors by type |
| `elasticetl_extract_requests_total` | Counter | Elasticsearch requests |
| `elasticetl_extract_duration_seconds` | Histogram | Extract phase duration |
| `elasticetl_transform_records_total` | Counter | Transformed records |
| `elasticetl_load_requests_total` | Counter | Load requests by stream type |
| `elasticetl_memory_usage_bytes` | Gauge | Memory consumption |
| `elasticetl_goroutines_active` | Gauge | Active goroutines |

### Health Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Overall health status |
| `/ready` | GET | Readiness probe |
| `/metrics` | GET | Prometheus metrics |

### Logging Configuration

```yaml
global:
  logging:
    level: "info"       # debug, info, warn, error
    format: "json"      # json, text
    output: "stdout"    # stdout, stderr, file path
```

### Pipeline Status Monitoring

```bash
# Check pipeline status
curl http://localhost:8080/status

# Example response
{
  "basic-metrics": {
    "name": "basic-metrics",
    "running": true,
    "enabled": true,
    "failed": false,
    "interval": "interval: 60s"
  }
}
```

## Stream Types

### Prometheus Stream

Send metrics to Prometheus pushgateway or remote write endpoint:

```yaml
- type: "prometheus"
  config:
    endpoint: "https://prometheus.example.com/api/v1/write"
    timeout: "30s"
  basic_auth:
    username: "${PROMETHEUS_USER}"
    password: "${PROMETHEUS_PASS}"
  labels:
    job: "elasticsearch-etl"
    environment: "production"
```

### OpenTelemetry Stream

Send metrics to OpenTelemetry collector:

```yaml
- type: "otel"
  config:
    endpoint: "http://otel-collector:4317"
    timeout: "30s"
  labels:
    service.name: "elasticsearch-etl"
    service.version: "1.0.0"
```

### GEM Stream

Send metrics to GEM with Prometheus remote write:

```yaml
- type: "gem"
  config:
    endpoint: "https://gem.example.com/api/v1/write"
    timeout: "30s"
  labels:
    source: "elasticsearch"
```

### CSV Stream

Export data to CSV files:

```yaml
- type: "csv"
  config:
    path: "/data/exports/metrics.csv"
```

### Debug Stream

Debug output with multiple formats:

```yaml
- type: "debug"
  config:
    path: "/tmp/debug/output"
    format: "json"  # json, prometheus, otel
```

## Data Transformation

### Type Conversions

```yaml
conversion_functions:
  - field: "_source.cpu_usage"
    function: "convert_type"
    from_type: "string"
    to_type: "float"
  - field: "_source.memory_bytes"
    function: "convert_to_mb"
    from_unit: "bytes"
```

### Supported Conversions

| Function | Description | Parameters |
|----------|-------------|------------|
| `convert_type` | Type conversion | `from_type`, `to_type` |
| `convert_to_kb` | Convert to kilobytes | `from_unit` |
| `convert_to_mb` | Convert to megabytes | `from_unit` |
| `convert_to_gb` | Convert to gigabytes | `from_unit` |

### Data Filtering

```yaml
filters:
  - type: "include"
    pattern: "^_source\\.(cpu|memory).*"
  - type: "exclude"
    pattern: ".*\\.keyword$"
```

### CSV Output Configuration

```yaml
transform:
  output_format: "csv"
  input:
    format: "json"     # Input format
    header: false      # CSV input has header
```

### Advanced CSV Processing

ElasticETL provides sophisticated CSV processing with:
- **Depth-based key analysis**: Sorts keys by depth for consistent CSV structure
- **Dynamic state tracking**: Creates CSV rows based on hierarchical data changes
- **Array handling**: Processes nested arrays and objects
- **Unique key generation**: Removes array indices for consistent column names

## Deployment

### Docker Deployment

```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o elasticetl ./cmd/elasticetl

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/elasticetl .
CMD ["./elasticetl", "--config", "/config/config.yaml"]
```

### Kubernetes Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: elasticetl
spec:
  replicas: 1
  selector:
    matchLabels:
      app: elasticetl
  template:
    metadata:
      labels:
        app: elasticetl
    spec:
      containers:
      - name: elasticetl
        image: elasticetl:latest
        resources:
          requests:
            memory: "256Mi"
            cpu: "250m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        env:
        - name: ES_TOKEN
          valueFrom:
            secretKeyRef:
              name: elasticsearch-secret
              key: token
        - name: PROMETHEUS_USER
          valueFrom:
            secretKeyRef:
              name: prometheus-secret
              key: username
        - name: PROMETHEUS_PASS
          valueFrom:
            secretKeyRef:
              name: prometheus-secret
              key: password
        volumeMounts:
        - name: config
          mountPath: /config
        - name: debug-output
          mountPath: /tmp/debug
      volumes:
      - name: config
        configMap:
          name: elasticetl-config
      - name: debug-output
        emptyDir: {}
```

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `ELASTICETL_CONFIG` | Configuration file path | `config.yaml` |
| `ELASTICETL_LOG_LEVEL` | Log level | `info` |
| `ELASTICETL_METRICS_PORT` | Metrics port | `8080` |

## Performance

### Performance Characteristics

| Metric | Value | Notes |
|--------|-------|-------|
| Max Pipelines | 100+ | Configurable limit |
| Max Concurrent Requests | 1000 | Per pipeline |
| Data Throughput | 10MB/s | Per pipeline |
| Query Response Time | <5s | 95th percentile |
| Memory Usage | <512MB | Typical workload |
| CPU Usage | <50% | Typical workload |

### Resource Configuration

```yaml
global:
  resource_limits:
    max_memory_mb: 512      # Memory limit
    max_cpu_percent: 50     # CPU limit
    max_goroutines: 1000    # Goroutine limit
```

### Performance Tuning

1. **Pipeline Intervals**: Balance data freshness with system load
2. **Concurrent Pipelines**: Monitor resource usage when scaling
3. **Query Optimization**: Use efficient Elasticsearch queries
4. **Stream Configuration**: Optimize timeout and retry settings
5. **Debug Output**: Disable debug in production for better performance

## Troubleshooting

### Common Issues

#### Pipeline Failures
```bash
# Check pipeline status
curl http://localhost:8080/status

# Check logs
tail -f /var/log/elasticetl.log | grep -E "(FAILED|ERROR)"

# Enable debug output
# Add debug configuration to extract and transform stages
```

#### Authentication Issues
```bash
# Test environment variables
echo $ES_TOKEN
echo $PROMETHEUS_USER

# Verify credentials
curl -H "Authorization: Bearer $ES_TOKEN" http://elasticsearch:9200/_cluster/health
```

#### Connection Timeouts
```yaml
# Increase timeouts
extract:
  timeout: "60s"
  max_retries: 5

load:
  streams:
    - type: "prometheus"
      config:
        timeout: "30s"
```

#### High Memory Usage
```yaml
# Reduce resource usage
transform:
  previous_results_sets: 1  # Reduce history
  stateless: true          # Enable stateless mode

global:
  resource_limits:
    max_memory_mb: 256     # Lower memory limit
```

### Debug Strategies

1. **Enable Debug Streams**: Use debug streams to inspect data flow
2. **Check Logs**: Review application logs for errors
3. **Verify Connectivity**: Test connections to external services
4. **Monitor Metrics**: Use Prometheus metrics to identify issues
5. **Validate Configuration**: Check YAML syntax and configuration values

### Log Analysis

```bash
# Filter by pipeline
grep "pipeline_name=basic-metrics" /var/log/elasticetl.log

# Filter by error level
grep "level=error" /var/log/elasticetl.log

# Monitor pipeline executions
grep "Pipeline.*executed" /var/log/elasticetl.log
```

## API Reference

### Command Line Interface

```bash
elasticetl [flags]

Flags:
  --config string     Configuration file path (default "config.yaml")
  --log-level string  Log level (debug, info, warn, error) (default "info")
  --metrics-port int  Metrics server port (default 8080)
  --help             Show help information
  --version          Show version information
```

### Configuration Examples

The project includes comprehensive configuration examples:

- **`configs/basic-config.yaml`**: Simple development configuration
- **`configs/production-config.yaml`**: Production-ready configuration
- **`configs/auth-example-config.yaml`**: Authentication examples
- **`configs/debug-formats-config.yaml`**: Debug format examples
- **`examples/simple-example.yaml`**: Minimal configuration
- **`examples/csv-flattened-config.yaml`**: CSV processing examples

### System Requirements

| Component | Minimum | Recommended | Maximum |
|-----------|---------|-------------|---------|
| Memory | 128MB | 512MB | 2GB |
| CPU | 1 Core | 2 Cores | 8 Cores |
| Disk Space | 100MB | 1GB | 10GB |
| Network | 1Mbps | 10Mbps | 100Mbps |

### Version Compatibility

| ElasticETL Version | Elasticsearch | Go Version | Kubernetes |
|-------------------|---------------|------------|------------|
| 1.0.x | 7.x, 8.x | 1.21+ | 1.20+ |

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests and documentation
5. Submit a pull request

## Support

For issues, questions, or contributions:
- Create an issue in the repository
- Review the comprehensive documentation
- Check configuration examples
- Monitor application logs and metrics

## License

[License information]

---

This consolidated documentation provides comprehensive coverage of ElasticETL's features, configuration options, and operational procedures. For specific implementation details, refer to the individual configuration files and technical specifications.
