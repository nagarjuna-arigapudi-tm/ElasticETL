# ElasticETL

A high-performance, production-ready ETL (Extract, Transform, Load) pipeline for processing Elasticsearch data and delivering it to various monitoring and analytics platforms.

## Features

- **Multi-Pipeline Support**: Run multiple ETL pipelines concurrently
- **Flexible Data Extraction**: Query Elasticsearch with custom DSL queries
- **Advanced Transformations**: Type conversions, regex filtering, CSV formatting
- **Multiple Output Streams**: Prometheus, OTEL, GEM, CSV, and debug outputs
- **Authentication & Security**: Bearer tokens, basic auth, TLS support
- **Environment Variables**: Secure credential management with `${VAR_NAME}` substitution
- **Resource Management**: Configurable memory, CPU, and connection limits
- **Hot Configuration Reload**: Update configurations without restart
- **Comprehensive Monitoring**: Built-in Prometheus metrics and health checks
- **Debug Capabilities**: Multiple debug formats for troubleshooting

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
    
    transform:
      stateless: true
      substitute_zeros_for_null: true
      output_format: "json"
    
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

## Configuration Examples

### Basic Configuration
- **File**: `configs/basic-config.yaml`
- **Use Case**: Development and learning
- **Features**: Simple pipeline, Prometheus output, debug stream

### Production Configuration
- **File**: `configs/production-config.yaml`
- **Use Case**: Production deployments
- **Features**: Multiple pipelines, authentication, failover, comprehensive monitoring

### Simple Example
- **File**: `examples/simple-example.yaml`
- **Use Case**: First-time users and testing
- **Features**: Minimal configuration, debug output only

### Authentication Example
- **File**: `configs/auth-example-config.yaml`
- **Use Case**: Secure environments
- **Features**: Environment variables, basic auth, bearer tokens

### Debug Formats
- **File**: `configs/debug-formats-config.yaml`
- **Use Case**: Troubleshooting
- **Features**: Multiple debug formats (JSON, Prometheus, OTEL)

## Supported Stream Types

| Stream Type | Description | Use Case |
|-------------|-------------|----------|
| `prometheus` | Prometheus pushgateway or remote write | Metrics collection |
| `otel` | OpenTelemetry collector | Observability platforms |
| `gem` | GEM with Prometheus remote write | GEM monitoring |
| `csv` | CSV file output | Data export and analysis |
| `debug` | Debug file output | Development and troubleshooting |

## Authentication

ElasticETL supports multiple authentication methods with environment variable substitution:

```yaml
# Bearer Token
auth_headers:
  - "Bearer ${ES_TOKEN}"

# Basic Authentication
basic_auth:
  username: "${PROMETHEUS_USER}"
  password: "${PROMETHEUS_PASS}"

# Custom Headers
additional_headers:
  - ["X-API-Key: ${API_KEY}", "X-Environment: production"]
```

## Debug Capabilities

ElasticETL provides comprehensive debugging through debug streams:

```yaml
# JSON Debug Output (default)
- type: "debug"
  config:
    path: "/tmp/debug/json-output"
    format: "json"

# Prometheus Format Debug
- type: "debug"
  config:
    path: "/tmp/debug/prometheus-output"
    format: "prometheus"

# OTEL Format Debug
- type: "debug"
  config:
    path: "/tmp/debug/otel-output"
    format: "otel"
```

## Monitoring

### Built-in Metrics

ElasticETL exposes Prometheus metrics on `/metrics` endpoint:

- Pipeline execution counts and durations
- Extract, transform, and load phase metrics
- Resource usage (memory, CPU, goroutines)
- Error rates and types

### Health Checks

- `/health` - Overall health status
- `/ready` - Readiness probe
- `/metrics` - Prometheus metrics

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `ELASTICETL_CONFIG` | Configuration file path | `config.yaml` |
| `ELASTICETL_LOG_LEVEL` | Log level | `info` |
| `ELASTICETL_METRICS_PORT` | Metrics port | `8080` |

## Command Line Options

```bash
elasticetl [flags]

Flags:
  --config string     Configuration file path (default "config.yaml")
  --log-level string  Log level (debug, info, warn, error) (default "info")
  --metrics-port int  Metrics server port (default 8080)
  --help             Show help information
  --version          Show version information
```

## Docker Deployment

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

## Kubernetes Deployment

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

## Documentation

- **[Configuration Guide](CONFIGURATION_GUIDE.md)** - Comprehensive configuration documentation
- **[Technical Specification](TECHNICAL_SPECIFICATION.md)** - Detailed technical specifications
- **Configuration Examples** - Available in `configs/` and `examples/` directories

## Architecture

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

## Performance

- **Throughput**: Up to 10MB/s per pipeline
- **Concurrency**: Support for 100+ concurrent pipelines
- **Memory Usage**: Typically <512MB
- **CPU Usage**: Typically <50%
- **Query Response**: <5s (95th percentile)

## System Requirements

| Component | Minimum | Recommended |
|-----------|---------|-------------|
| Memory | 128MB | 512MB |
| CPU | 1 Core | 2 Cores |
| Disk Space | 100MB | 1GB |
| Network | 1Mbps | 10Mbps |

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## License

[License information]

## Support

For issues, questions, or contributions:
- Create an issue in the repository
- Check the documentation in `CONFIGURATION_GUIDE.md` and `TECHNICAL_SPECIFICATION.md`
- Review configuration examples in `configs/` and `examples/` directories

## Version Compatibility

| ElasticETL Version | Elasticsearch | Go Version |
|-------------------|---------------|------------|
| 1.0.x | 7.x, 8.x | 1.21+ |
