# ElasticETL

A flexible and powerful ETL (Extract, Transform, Load) tool designed specifically for Elasticsearch data processing with support for multiple output streams including Prometheus, OpenTelemetry, and GEM.

## Features

### Core Capabilities
- **Multiple ETL Pipelines**: Run multiple independent pipelines simultaneously
- **Hot Configuration Reload**: Update configuration without restarting the application
- **Resource Management**: Built-in resource limits and monitoring
- **Comprehensive Metrics**: Per-pipeline and system-wide metrics collection
- **Flexible Data Transformation**: Support for type conversion, unit conversion, and custom functions
- **Multiple Output Streams**: Support for Prometheus, OpenTelemetry, and GEM outputs
- **Query Macros**: Dynamic query substitution with time and cluster macros

### Extract Features
- **Elasticsearch Integration**: Native support for Elasticsearch queries
- **Multiple Endpoints**: Query multiple Elasticsearch clusters with cluster-specific configurations
- **Raw Query Support**: Use raw Elasticsearch queries without JSON escaping
- **Query Macros**: Support for `__STARTTIME__`, `__ENDTIME__`, and `__CLUSTER__` macros
- **JSON Path Extraction**: Extract specific fields using JSON path expressions
- **Retry Logic**: Configurable retry mechanism with exponential backoff
- **Concurrent Processing**: Parallel extraction from multiple endpoints

### Query Macros
ElasticETL supports dynamic macro substitution in queries:

- **`__STARTTIME__`**: Substituted with start time (unix timestamp in milliseconds)
- **`__ENDTIME__`**: Substituted with end time (unix timestamp in milliseconds)  
- **`__CLUSTER__`**: Substituted with the cluster name from endpoint configuration

#### Time Expression Formats
- `NOW` - Current timestamp
- `NOW-5min` - 5 minutes ago
- `NOW+10sec` - 10 seconds from now
- `1640995200000` - Direct unix timestamp

### Transform Features
- **Stateless/Stateful Processing**: Choose between stateless or stateful transformations
- **Null Handling**: Automatic substitution of zeros for null/missing values
- **Historical Data**: Store and access previous transformation results
- **Type Conversion**: Convert between different data types (string, int, float, bool)
- **Unit Conversion**: Convert between different units (bytes, KB, MB, GB)
- **Custom Functions**: Extensible transformation function system

### Load Features
- **Multiple Streams**: Send data to multiple destinations simultaneously
- **Prometheus Support**: Native Prometheus remote write and pushgateway support
- **OpenTelemetry Integration**: Send metrics to OTEL collectors
- **GEM Integration**: Support for GEM with Prometheus remote write
- **Concurrent Loading**: Parallel loading to multiple streams

## Installation

### Prerequisites
- Go 1.19 or later
- Access to Elasticsearch cluster(s)
- Target systems (Prometheus, OTEL collector, GEM) if using those outputs

### Build from Source
```bash
git clone <repository-url>
cd ElasticETL
go mod tidy
go build -o elasticetl ./cmd/elasticetl
```

## Configuration

ElasticETL uses JSON or YAML configuration files. The configuration consists of:

### Pipeline Configuration
Each pipeline defines:
- **Extract Config**: Elasticsearch query, endpoints with cluster names, JSON paths, intervals, time macros
- **Transform Config**: Transformation rules, conversion functions, state management
- **Load Config**: Output streams and their configurations

### Global Configuration
- **Resource Limits**: Memory, CPU, goroutine, and connection limits
- **Metrics**: Metrics collection and HTTP server settings
- **Logging**: Log level, format, and output configuration

### Example Configuration
```json
{
  "pipelines": [
    {
      "name": "elasticsearch-metrics",
      "enabled": true,
      "interval": "5m",
      "extract": {
        "elasticsearch_query": "{\n  \"query\": {\n    \"bool\": {\n      \"must\": [\n        {\n          \"range\": {\n            \"@timestamp\": {\n              \"gte\": __STARTTIME__,\n              \"lte\": __ENDTIME__\n            }\n          }\n        },\n        {\n          \"term\": {\n            \"cluster.name\": \"__CLUSTER__\"\n          }\n        }\n      ]\n    }\n  },\n  \"aggs\": {\n    \"avg_response_time\": {\n      \"avg\": {\n        \"field\": \"response_time\"\n      }\n    }\n  }\n}",
        "endpoints": [
          {
            "url": "http://localhost:9200/logs-*",
            "cluster_name": "production"
          }
        ],
        "json_paths": [
          "aggregations.avg_response_time.value"
        ],
        "timeout": "30s",
        "max_retries": 3,
        "start_time": "NOW-5min",
        "end_time": "NOW"
      },
      "transform": {
        "stateless": false,
        "substitute_zeros_for_null": true,
        "previous_results_sets": 5,
        "conversion_functions": [
          {
            "field": "response_time",
            "function": "convert_type",
            "from_type": "string",
            "to_type": "float"
          }
        ]
      },
      "load": {
        "streams": [
          {
            "type": "prometheus",
            "config": {
              "endpoint": "http://localhost:9091/metrics/job/elasticetl"
            }
          }
        ]
      }
    }
  ],
  "global": {
    "resource_limits": {
      "max_memory_mb": 512,
      "max_cpu_percent": 80
    },
    "metrics": {
      "enabled": true,
      "port": 8090,
      "path": "/metrics"
    }
  }
}
```

## Usage

### Basic Usage
```bash
# Run with default configuration
./elasticetl

# Run with custom configuration
./elasticetl -config /path/to/config.json

# Set log level
./elasticetl -log-level debug

# Show version
./elasticetl -version
```

### Hot Configuration Reload
ElasticETL automatically detects configuration file changes and reloads without restart:
1. Modify your configuration file
2. Save the changes
3. ElasticETL will automatically reload and apply the new configuration

### Query Macros Usage

#### Time Macros
```json
{
  "elasticsearch_query": "{\n  \"query\": {\n    \"range\": {\n      \"@timestamp\": {\n        \"gte\": __STARTTIME__,\n        \"lte\": __ENDTIME__\n      }\n    }\n  }\n}",
  "start_time": "NOW-1min",
  "end_time": "NOW"
}
```

#### Cluster Macro
```json
{
  "elasticsearch_query": "{\n  \"query\": {\n    \"term\": {\n      \"cluster.name\": \"__CLUSTER__\"\n    }\n  }\n}",
  "endpoints": [
    {
      "url": "http://localhost:9200/logs-*",
      "cluster_name": "production"
    }
  ]
}
```

### Monitoring and Metrics

#### Built-in Metrics Server
ElasticETL exposes metrics via HTTP endpoints:

- `GET /metrics` - All metrics (system + pipelines)
- `GET /metrics/system` - System metrics only
- `GET /metrics/pipeline/{name}` - Specific pipeline metrics

#### Available Metrics
**System Metrics:**
- Total/used memory
- CPU usage
- Active goroutines
- Pipeline counts
- Uptime

**Pipeline Metrics:**
- Execution counts (total, successful, failed)
- Processing times
- Data volume (entries, bytes processed)
- Error rates
- Resource usage per pipeline

### Transformation Functions

#### Type Conversion
```json
{
  "field": "count",
  "function": "convert_type",
  "from_type": "string",
  "to_type": "int"
}
```

#### Unit Conversion
```json
{
  "field": "memory_usage",
  "function": "convert_to_mb",
  "from_unit": "bytes"
}
```

Supported conversions:
- `convert_to_kb` - Convert to kilobytes
- `convert_to_mb` - Convert to megabytes  
- `convert_to_gb` - Convert to gigabytes

### Output Stream Types

#### Prometheus
```json
{
  "type": "prometheus",
  "config": {
    "endpoint": "http://localhost:9091/metrics/job/elasticetl/instance/localhost",
    "timeout": "30s"
  }
}
```

#### OpenTelemetry
```json
{
  "type": "otel",
  "config": {
    "endpoint": "http://localhost:4318/v1/metrics",
    "timeout": "30s"
  }
}
```

#### GEM (Grafana Enterprise Metrics)
```json
{
  "type": "gem",
  "config": {
    "endpoint": "http://localhost:8080/api/v1/write",
    "timeout": "30s"
  }
}
```

## Architecture

ElasticETL follows a modular architecture:

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Config        │    │   Pipeline       │    │   Metrics       │
│   Loader        │───▶│   Manager        │───▶│   Collector     │
│   (Hot Reload)  │    │                  │    │                 │
└─────────────────┘    └──────────────────┘    └─────────────────┘
                              │
                              ▼
                    ┌──────────────────┐
                    │   Individual     │
                    │   Pipelines      │
                    └──────────────────┘
                              │
                              ▼
        ┌─────────────┬───────────────┬─────────────┐
        │             │               │             │
        ▼             ▼               ▼             ▼
   ┌─────────┐  ┌─────────────┐  ┌─────────┐  ┌─────────┐
   │Extract  │  │ Transform   │  │  Load   │  │Metrics  │
   │Component│  │ Component   │  │Component│  │Component│
   └─────────┘  └─────────────┘  └─────────┘  └─────────┘
```

### Components

1. **Config Loader**: Handles configuration loading and hot reload
2. **Pipeline Manager**: Manages multiple pipeline instances
3. **Extract Component**: Handles data extraction from Elasticsearch with macro substitution
4. **Transform Component**: Processes and transforms extracted data
5. **Load Component**: Sends transformed data to output streams
6. **Metrics Collector**: Collects and exposes system and pipeline metrics

## Resource Management

ElasticETL includes built-in resource management:

- **Memory Limits**: Configurable maximum memory usage
- **CPU Limits**: CPU usage monitoring and alerting
- **Goroutine Limits**: Control concurrent operations
- **Connection Limits**: Manage HTTP connections to external systems

## Error Handling and Resilience

- **Retry Logic**: Configurable retry mechanisms with exponential backoff
- **Circuit Breaker**: Automatic failure detection and recovery
- **Graceful Degradation**: Continue operating even if some components fail
- **Comprehensive Logging**: Detailed error reporting and debugging information

## Performance Considerations

- **Concurrent Processing**: Parallel extraction, transformation, and loading
- **Efficient Memory Usage**: Streaming processing to minimize memory footprint
- **Connection Pooling**: Reuse HTTP connections for better performance
- **Configurable Intervals**: Adjust processing frequency based on requirements
- **Macro Substitution**: Dynamic query generation at runtime for optimal performance

## Troubleshooting

### Common Issues

1. **Configuration Errors**: Check JSON syntax and required fields
2. **Connection Issues**: Verify Elasticsearch and output stream endpoints
3. **Memory Issues**: Adjust resource limits and processing intervals
4. **Performance Issues**: Monitor metrics and adjust concurrency settings
5. **Macro Issues**: Validate time expressions and cluster names

### Debug Mode
Run with debug logging for detailed information:
```bash
./elasticetl -log-level debug
```

### Metrics Monitoring
Monitor the metrics endpoint for system health:
```bash
curl http://localhost:8090/metrics
```

## Examples

### Basic Log Analysis Pipeline
```json
{
  "name": "log-analysis",
  "enabled": true,
  "interval": "1m",
  "extract": {
    "elasticsearch_query": "{\n  \"query\": {\n    \"bool\": {\n      \"must\": [\n        {\n          \"range\": {\n            \"@timestamp\": {\n              \"gte\": __STARTTIME__\n            }\n          }\n        },\n        {\n          \"term\": {\n            \"cluster.name\": \"__CLUSTER__\"\n          }\n        }\n      ]\n    }\n  },\n  \"aggs\": {\n    \"error_count\": {\n      \"filter\": {\n        \"term\": {\n          \"level\": \"ERROR\"\n        }\n      }\n    }\n  }\n}",
    "endpoints": [
      {
        "url": "http://localhost:9200/logs-*",
        "cluster_name": "production"
      }
    ],
    "json_paths": ["aggregations.error_count.doc_count"],
    "start_time": "NOW-1min"
  }
}
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## License

[Add your license information here]

## Support

[Add support information here]
