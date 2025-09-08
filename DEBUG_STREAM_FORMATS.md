# Debug Stream Format Guide

The ElasticETL debug stream now supports multiple output formats to help you debug and inspect the data being processed by different load streams. This feature allows you to see exactly what data would be sent to Prometheus or OTEL collectors without actually sending it.

## Supported Formats

### 1. JSON Format (Default)
The default format that outputs comprehensive debug information in JSON format.

**Configuration:**
```yaml
- type: "debug"
  config:
    path: "/tmp/elasticetl/debug/json-output"
    format: "json"  # Optional - this is the default
```

**Output Example:**
```json
{
  "timestamp": "2025-09-08T04:35:00Z",
  "pipeline": "load",
  "format": "json",
  "results_count": 2,
  "results": [
    {
      "timestamp": "2025-09-08T04:35:00Z",
      "source": "https://elasticsearch.example.com:9200",
      "data": {
        "cpu_usage": 75.5,
        "memory_usage": 1024
      },
      "metadata": {
        "cluster_name": "production-cluster",
        "endpoint": "https://elasticsearch.example.com:9200"
      }
    }
  ]
}
```

**File Extension:** `.json`

### 2. Prometheus Format
Outputs data in the same format that would be sent to a Prometheus pushgateway or remote write endpoint.

**Configuration:**
```yaml
- type: "debug"
  config:
    path: "/tmp/elasticetl/debug/prometheus-output"
    format: "prometheus"
```

**Output Example:**
```
# ElasticETL Debug Output - Prometheus Format
# Generated at: 2025-09-08T04:35:00Z
# Results count: 2

cpu_usage{source="https://elasticsearch.example.com:9200",cluster="production-cluster"} 75.500000 1725768900000
memory_usage{source="https://elasticsearch.example.com:9200",cluster="production-cluster"} 1024.000000 1725768900000
```

**File Extension:** `.txt`

### 3. OTEL Format
Outputs data in the OpenTelemetry collector format that would be sent to an OTEL endpoint.

**Configuration:**
```yaml
- type: "debug"
  config:
    path: "/tmp/elasticetl/debug/otel-output"
    format: "otel"
```

**Output Example:**
```json
{
  "timestamp": "2025-09-08T04:35:00Z",
  "pipeline": "load",
  "format": "otel",
  "resourceMetrics": [
    {
      "resource": {
        "attributes": [
          {
            "key": "service.name",
            "value": {
              "stringValue": "elasticetl"
            }
          }
        ]
      },
      "scopeMetrics": [
        {
          "scope": {
            "name": "elasticetl",
            "version": "1.0.0"
          },
          "metrics": [
            {
              "name": "elasticetl_metric",
              "description": "Metric from ElasticETL",
              "unit": "1",
              "data": {
                "dataPoints": [
                  {
                    "attributes": {
                      "source": "https://elasticsearch.example.com:9200",
                      "cluster": "production-cluster"
                    },
                    "timeUnixNano": 1725768900000000000,
                    "value": {
                      "cpu_usage": 75.5,
                      "memory_usage": 1024
                    }
                  }
                ]
              }
            }
          ]
        }
      ]
    }
  ]
}
```

**File Extension:** `.json`

## Use Cases

### 1. Development and Testing
Use debug streams to verify that your ETL pipeline is processing data correctly before sending it to production endpoints:

```yaml
load:
  streams:
    # Debug what would be sent to Prometheus
    - type: "debug"
      config:
        path: "/tmp/debug/prometheus-data"
        format: "prometheus"
    
    # Debug what would be sent to OTEL
    - type: "debug"
      config:
        path: "/tmp/debug/otel-data"
        format: "otel"
```

### 2. Troubleshooting Data Issues
When you're experiencing issues with data not appearing correctly in your monitoring systems, use debug streams to inspect the exact data being generated:

```yaml
load:
  streams:
    # Regular production stream
    - type: "prometheus"
      config:
        endpoint: "https://prometheus.prod.com/api/v1/write"
      basic_auth:
        username: "${PROM_USER}"
        password: "${PROM_PASS}"
    
    # Debug stream to see what's being sent
    - type: "debug"
      config:
        path: "/tmp/debug/prometheus-troubleshoot"
        format: "prometheus"
```

### 3. Format Comparison
Compare how the same data looks in different formats:

```yaml
load:
  streams:
    - type: "debug"
      config:
        path: "/tmp/debug/data-json"
        format: "json"
    
    - type: "debug"
      config:
        path: "/tmp/debug/data-prometheus"
        format: "prometheus"
    
    - type: "debug"
      config:
        path: "/tmp/debug/data-otel"
        format: "otel"
```

## File Naming Convention

Debug files are automatically named with timestamps to prevent overwrites:

- **JSON format:** `{path}_load_{timestamp}.json`
- **Prometheus format:** `{path}_load_{timestamp}.txt`
- **OTEL format:** `{path}_load_{timestamp}.json`

Example filenames:
- `debug_load_20250908_043500.json`
- `prometheus-output_load_20250908_043500.txt`
- `otel-data_load_20250908_043500.json`

## Configuration Reference

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `path` | string | Yes | - | Base path for debug output files |
| `format` | string | No | `"json"` | Output format: `"json"`, `"prometheus"`, or `"otel"` |

## Best Practices

1. **Use Temporary Directories:** Store debug files in `/tmp` or similar temporary locations to avoid cluttering your filesystem.

2. **Clean Up Regularly:** Debug files can accumulate quickly. Set up log rotation or cleanup scripts.

3. **Secure Sensitive Data:** Debug files may contain sensitive metrics data. Ensure appropriate file permissions and cleanup.

4. **Development vs Production:** Use debug streams primarily in development and testing environments. In production, use them sparingly for troubleshooting.

5. **Monitor Disk Usage:** Debug files can consume significant disk space, especially with high-frequency pipelines.

## Example Complete Configuration

```yaml
pipelines:
  - name: "debug-example"
    enabled: true
    interval: "60s"
    
    extract:
      elasticsearch_query: '{"query": {"match_all": {}}, "size": 10}'
      urls: ["https://elasticsearch.example.com:9200"]
      cluster_names: ["test-cluster"]
      json_path: "hits.hits._source"
      timeout: "30s"
      max_retries: 3
    
    transform:
      stateless: true
      substitute_zeros_for_null: true
      output_format: "json"
    
    load:
      streams:
        # Production streams
        - type: "prometheus"
          config:
            endpoint: "https://prometheus.prod.com/api/v1/write"
          basic_auth:
            username: "${PROM_USER}"
            password: "${PROM_PASS}"
        
        - type: "otel"
          config:
            endpoint: "https://otel.prod.com/v1/metrics"
        
        # Debug streams for troubleshooting
        - type: "debug"
          config:
            path: "/tmp/elasticetl/debug/prometheus-debug"
            format: "prometheus"
        
        - type: "debug"
          config:
            path: "/tmp/elasticetl/debug/otel-debug"
            format: "otel"
        
        - type: "debug"
          config:
            path: "/tmp/elasticetl/debug/full-debug"
            format: "json"
```

This configuration will generate debug files showing exactly what data is being sent to your Prometheus and OTEL endpoints, making it easy to troubleshoot any data formatting or processing issues.
