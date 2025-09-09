# ElasticETL Technical Specification

## Overview

ElasticETL is a high-performance, production-ready ETL (Extract, Transform, Load) pipeline designed specifically for processing Elasticsearch data and delivering it to various monitoring and analytics platforms.

## Architecture

### Core Components

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│    EXTRACT      │───▶│   TRANSFORM     │───▶│      LOAD       │
│                 │    │                 │    │                 │
│ • Elasticsearch │    │ • Data Mapping  │    │ • Prometheus    │
│ • JSON Path     │    │ • Type Convert  │    │ • OTEL          │
│ • Filtering     │    │ • CSV Format    │    │ • GEM           │
│ • Auth/TLS      │    │ • Regex Match   │    │ • CSV Files     │
│ • Retry Logic   │    │ • Null Handling │    │ • Debug Output  │
└─────────────────┘    └─────────────────┘    └─────────────────┘
```

### Pipeline Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│                        ElasticETL Core                           │
├──────────────────────────────────────────────────────────────────┤
│  Pipeline Manager                                                │
│  ├─ Pipeline 1 (Extract → Transform → Load)                     │
│  ├─ Pipeline 2 (Extract → Transform → Load)                     │
│  └─ Pipeline N (Extract → Transform → Load)                     │
├──────────────────────────────────────────────────────────────────┤
│  Resource Manager                                                │
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

## Technical Specifications

### System Requirements

| Component | Minimum | Recommended | Maximum |
|-----------|---------|-------------|---------|
| Memory | 128MB | 512MB | 2GB |
| CPU | 1 Core | 2 Cores | 8 Cores |
| Disk Space | 100MB | 1GB | 10GB |
| Network | 1Mbps | 10Mbps | 100Mbps |

### Performance Characteristics

| Metric | Value | Notes |
|--------|-------|-------|
| Max Pipelines | 100 | Configurable limit |
| Max Concurrent Requests | 1000 | Per pipeline |
| Data Throughput | 10MB/s | Per pipeline |
| Query Response Time | <5s | 95th percentile |
| Memory Usage | <512MB | Typical workload |
| CPU Usage | <50% | Typical workload |

## Configuration Schema

### Pipeline Configuration

```yaml
pipelines:
  - name: string                    # Pipeline identifier
    enabled: boolean                # Enable/disable pipeline
    interval: duration              # Execution interval (e.g., "30s", "5m")
    
    extract:
      elasticsearch_query: string   # Elasticsearch DSL query
      urls: []string                # Elasticsearch endpoints
      cluster_names: []string       # Cluster identifiers
      auth_headers: []string        # Authentication headers
      additional_headers: [][]string # Custom headers
      json_path: string             # JSON extraction path
      filters: []FilterConfig       # Field filters
      timeout: duration             # Request timeout
      max_retries: int              # Retry attempts
      insecure_tls: boolean         # Skip TLS verification
      debug: DebugConfig            # Debug configuration
    
    transform:
      stateless: boolean            # Stateless processing
      substitute_zeros_for_null: boolean # Null handling
      previous_results_sets: int    # History retention
      output_format: string         # "json" or "csv"
      conversion_functions: []ConversionFunctionConfig
    
    load:
      streams: []StreamConfig       # Output destinations
```

### Stream Types

#### Prometheus Stream
```yaml
- type: "prometheus"
  config:
    endpoint: string                # Pushgateway or remote write URL
    timeout: duration               # Request timeout
  basic_auth:
    username: string                # Basic auth username
    password: string                # Basic auth password
  insecure_tls: boolean            # Skip TLS verification
  labels:                          # Static labels
    key: value
```

#### OTEL Stream
```yaml
- type: "otel"
  config:
    endpoint: string                # OTEL collector endpoint
    timeout: duration               # Request timeout
  insecure_tls: boolean            # Skip TLS verification
  labels:                          # Resource attributes
    key: value
```

#### Debug Stream
```yaml
- type: "debug"
  config:
    path: string                    # Output file path
    format: string                  # "json", "prometheus", "otel"
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
  insecure_tls: boolean            # Skip TLS verification
  labels:                          # Static labels
    key: value
```

## Data Flow

### Extract Phase

1. **Query Execution**
   - Execute Elasticsearch DSL query against configured endpoints
   - Handle authentication (Bearer tokens, Basic auth)
   - Apply custom headers
   - Implement retry logic with exponential backoff

2. **Data Extraction**
   - Extract data using JSON path expressions
   - Flatten nested JSON structures
   - Apply include/exclude filters using regex patterns

3. **Error Handling**
   - Connection failures with failover
   - Query timeouts with retries
   - Authentication errors
   - Rate limiting

### Transform Phase

1. **Data Processing**
   - Type conversions (string → int64, float64, etc.)
   - Unit conversions (bytes → KB/MB/GB)
   - Null value substitution
   - Regex-based field matching

2. **Output Formatting**
   - JSON format (default)
   - CSV format with flattened structure
   - Depth-based unique key generation

3. **Data Validation**
   - Schema validation
   - Type checking
   - Range validation

### Load Phase

1. **Stream Processing**
   - Concurrent loading to multiple destinations
   - Format-specific serialization
   - Authentication handling
   - Error recovery

2. **Output Formats**
   - Prometheus exposition format
   - OTEL metrics format
   - CSV files with timestamps
   - Debug outputs (JSON, Prometheus, OTEL)

## Security Features

### Authentication

| Method | Support | Environment Variables |
|--------|---------|----------------------|
| Bearer Token | ✅ | `${TOKEN_VAR}` |
| Basic Auth | ✅ | `${USER}`, `${PASS}` |
| Custom Headers | ✅ | `${HEADER_VAR}` |
| API Keys | ✅ | `${API_KEY}` |

### TLS Configuration

| Feature | Support | Configuration |
|---------|---------|---------------|
| TLS 1.2+ | ✅ | Default |
| Certificate Validation | ✅ | Default |
| Insecure TLS | ✅ | `insecure_tls: true` |
| Custom CA | ❌ | Future enhancement |

### Environment Variable Substitution

Pattern: `${VARIABLE_NAME}`

Examples:
```yaml
auth_headers:
  - "Bearer ${ES_TOKEN}"
basic_auth:
  username: "${PROMETHEUS_USER}"
  password: "${PROMETHEUS_PASS}"
```

## Monitoring and Observability

### Built-in Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `elasticetl_pipeline_executions_total` | Counter | Total pipeline executions |
| `elasticetl_pipeline_duration_seconds` | Histogram | Pipeline execution time |
| `elasticetl_pipeline_errors_total` | Counter | Pipeline errors |
| `elasticetl_extract_requests_total` | Counter | Elasticsearch requests |
| `elasticetl_extract_duration_seconds` | Histogram | Extract phase duration |
| `elasticetl_transform_records_total` | Counter | Transformed records |
| `elasticetl_load_requests_total` | Counter | Load requests by stream |
| `elasticetl_memory_usage_bytes` | Gauge | Memory consumption |
| `elasticetl_goroutines_active` | Gauge | Active goroutines |

### Health Checks

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Overall health status |
| `/ready` | GET | Readiness probe |
| `/metrics` | GET | Prometheus metrics |

### Logging

| Level | Usage |
|-------|-------|
| `debug` | Detailed execution traces |
| `info` | Normal operations |
| `warn` | Non-critical issues |
| `error` | Critical errors |

Log formats:
- `json`: Structured JSON logs
- `text`: Human-readable text

## Error Handling

### Retry Strategy

| Component | Strategy | Max Retries | Backoff |
|-----------|----------|-------------|---------|
| Elasticsearch | Exponential | 3 | 1s, 2s, 4s |
| Prometheus | Linear | 2 | 1s, 2s |
| OTEL | Linear | 2 | 1s, 2s |

### Circuit Breaker

| Parameter | Value | Description |
|-----------|-------|-------------|
| Failure Threshold | 5 | Consecutive failures |
| Recovery Timeout | 30s | Time before retry |
| Half-Open Requests | 3 | Test requests |

## Resource Management

### Memory Management

- Configurable memory limits per pipeline
- Automatic garbage collection
- Memory usage monitoring
- OOM protection

### CPU Management

- Configurable CPU limits
- Goroutine pool management
- CPU usage monitoring
- Load balancing

### Connection Management

- HTTP connection pooling
- Configurable connection limits
- Connection reuse
- Timeout management

## Configuration Management

### Hot Reload

- Configuration file watching
- Graceful pipeline restart
- Zero-downtime updates
- Rollback capability

### Validation

- Schema validation on startup
- Runtime configuration checks
- Environment variable validation
- Connectivity testing

## Deployment Considerations

### Container Deployment

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
        volumeMounts:
        - name: config
          mountPath: /config
      volumes:
      - name: config
        configMap:
          name: elasticetl-config
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

### Configuration File Format

Supported formats:
- YAML (recommended)
- JSON

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `ELASTICETL_CONFIG` | Configuration file path | `config.yaml` |
| `ELASTICETL_LOG_LEVEL` | Log level | `info` |
| `ELASTICETL_METRICS_PORT` | Metrics port | `8080` |

## Version Compatibility

| ElasticETL Version | Elasticsearch | Go Version | Kubernetes |
|-------------------|---------------|------------|------------|
| 1.0.x | 7.x, 8.x | 1.21+ | 1.20+ |

## Future Enhancements

### Planned Features

- [ ] Custom CA certificate support
- [ ] Webhook notifications
- [ ] Data encryption at rest
- [ ] Advanced filtering DSL
- [ ] Multi-tenant support
- [ ] Grafana dashboard templates
- [ ] Alerting rules
- [ ] Data lineage tracking

### Performance Improvements

- [ ] Parallel query execution
- [ ] Result caching
- [ ] Compression support
- [ ] Batch processing optimization
- [ ] Memory pool optimization

## Support and Maintenance

### Troubleshooting

1. **High Memory Usage**
   - Reduce `previous_results_sets`
   - Lower `max_goroutines`
   - Enable debug logging

2. **Connection Timeouts**
   - Increase `timeout` values
   - Check network connectivity
   - Verify authentication

3. **Data Processing Errors**
   - Enable debug streams
   - Check JSON path expressions
   - Validate conversion functions

### Performance Tuning

1. **Optimize Query Intervals**
   - Balance freshness vs. load
   - Use appropriate aggregation periods
   - Consider data volume

2. **Resource Allocation**
   - Monitor CPU and memory usage
   - Adjust limits based on workload
   - Scale horizontally if needed

3. **Network Optimization**
   - Use connection pooling
   - Enable compression
   - Optimize query size

This technical specification provides comprehensive coverage of ElasticETL's architecture, configuration, and operational characteristics for development, deployment, and maintenance teams.
