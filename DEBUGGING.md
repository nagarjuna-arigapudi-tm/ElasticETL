# ElasticETL Debugging Guide

This guide provides comprehensive instructions on how to debug ElasticETL pipelines and troubleshoot common issues.

## Table of Contents
1. [Debug Configuration](#debug-configuration)
2. [Extract Phase Debugging](#extract-phase-debugging)
3. [Load Phase Debugging](#load-phase-debugging)
4. [Application-Level Debugging](#application-level-debugging)
5. [Common Issues and Solutions](#common-issues-and-solutions)
6. [Debug Output Analysis](#debug-output-analysis)
7. [Performance Debugging](#performance-debugging)

## Debug Configuration

### 1. Extract Phase Debug

Enable debug output after the extraction phase to inspect raw data from Elasticsearch:

```yaml
pipelines:
  - name: my-pipeline
    extract:
      # ... other extract config
      debug:
        enabled: true
        path: /tmp/elasticetl/debug/extract
```

**What it does:**
- Creates timestamped JSON files with extraction results
- Shows raw data extracted from Elasticsearch
- Includes metadata like cluster names, query details, and response sizes
- Helps identify issues with Elasticsearch queries or data extraction

### 2. Load Phase Debug

Add a debug stream to output transformed data to files:

```yaml
pipelines:
  - name: my-pipeline
    load:
      streams:
        - type: debug
          config:
            path: /tmp/elasticetl/debug/load
        # ... other streams
```

**What it does:**
- Creates timestamped JSON files with transformed data
- Shows data after transformation but before sending to external systems
- Helps verify transformation logic and data format

### 3. Application Debug Logging

Run ElasticETL with debug logging enabled:

```bash
./elasticetl -config configs/config.yaml -log-level debug
```

**Log levels available:**
- `debug`: Most verbose, shows all operations
- `info`: Standard operational information
- `warn`: Warning messages only
- `error`: Error messages only

## Extract Phase Debugging

### Step-by-Step Process

1. **Enable extract debug in your configuration:**
```yaml
extract:
  elasticsearch_query: |
    {
      "query": {
        "bool": {
          "must": [
            {
              "range": {
                "@timestamp": {
                  "gte": __STARTTIME__,
                  "lte": __ENDTIME__
                }
              }
            },
            {
              "term": {
                "cluster.name": "__CLUSTER__"
              }
            }
          ]
        }
      },
      "aggs": {
        "avg_response_time": {
          "avg": {
            "field": "response_time"
          }
        }
      }
    }
  urls:
    - http://localhost:9200/logs-*
  cluster_names:
    - production
  json_paths:
    - aggregations.avg_response_time.value
  debug:
    enabled: true
    path: /tmp/elasticetl/debug/extract
```

2. **Run ElasticETL:**
```bash
./elasticetl -config configs/config.yaml -log-level debug
```

3. **Check debug output:**
```bash
ls -la /tmp/elasticetl/debug/extract/
cat /tmp/elasticetl/debug/extract/extract_extract_20240830_235959.json
```

### Extract Debug Output Format

```json
{
  "timestamp": "2024-08-30T23:59:59Z",
  "pipeline": "extract",
  "results_count": 1,
  "results": [
    {
      "timestamp": "2024-08-30T23:59:59Z",
      "source": "http://localhost:9200/logs-*",
      "data": {
        "value": 125.5
      },
      "metadata": {
        "endpoint": "http://localhost:9200/logs-*",
        "cluster_name": "production",
        "query": "{ \"query\": { \"bool\": { \"must\": [...] } } }",
        "original_query": "{ \"query\": { \"bool\": { \"must\": [...] } } }",
        "response_size": 1024
      }
    }
  ]
}
```

### What to Look For in Extract Debug

- **Query Substitution**: Check if `__STARTTIME__`, `__ENDTIME__`, and `__CLUSTER__` macros were properly substituted
- **Response Data**: Verify the extracted data matches your expectations
- **JSON Path Results**: Ensure JSON paths are extracting the correct values
- **Metadata**: Check cluster names, endpoints, and response sizes

## Load Phase Debugging

### Step-by-Step Process

1. **Add debug stream to your configuration:**
```yaml
load:
  streams:
    - type: debug
      config:
        path: /tmp/elasticetl/debug/load
    - type: prometheus
      config:
        endpoint: http://localhost:9091/metrics/job/elasticetl
      labels:
        environment: production
```

2. **Run ElasticETL and check output:**
```bash
./elasticetl -config configs/config.yaml -log-level debug
ls -la /tmp/elasticetl/debug/load/
cat /tmp/elasticetl/debug/load/load_load_20240830_235959.json
```

### Load Debug Output Format

```json
{
  "timestamp": "2024-08-30T23:59:59Z",
  "pipeline": "load",
  "results_count": 1,
  "results": [
    {
      "timestamp": "2024-08-30T23:59:59Z",
      "source": "http://localhost:9200/logs-*",
      "data": {
        "value": 125.5
      },
      "metadata": {
        "endpoint": "http://localhost:9200/logs-*",
        "cluster_name": "production",
        "query": "...",
        "response_size": 1024
      },
      "transformed_data": {
        "value": 125.5
      }
    }
  ]
}
```

### What to Look For in Load Debug

- **Transformed Data**: Verify transformations were applied correctly
- **Data Types**: Check if type conversions worked as expected
- **Unit Conversions**: Verify unit conversions (KB, MB, GB) are correct
- **Labels**: Confirm cluster names and custom labels are present

## Application-Level Debugging

### 1. Enable Debug Logging

```bash
# Run with debug logging
./elasticetl -config configs/config.yaml -log-level debug

# Run with specific log format
./elasticetl -config configs/config.yaml -log-level debug 2>&1 | tee debug.log
```

### 2. Monitor Metrics Endpoint

```bash
# Check system metrics
curl http://localhost:8090/metrics

# Check specific pipeline metrics
curl http://localhost:8090/metrics/pipeline/my-pipeline

# Monitor continuously
watch -n 5 'curl -s http://localhost:8090/metrics/system'
```

### 3. Configuration Validation

```bash
# Test configuration without running
./elasticetl -config configs/config.yaml -validate-only

# Check configuration syntax
./elasticetl -config configs/config.yaml -dry-run
```

## Common Issues and Solutions

### 1. No Data Extracted

**Symptoms:**
- Extract debug shows empty results
- No files created in debug directories

**Debug Steps:**
```bash
# Check Elasticsearch connectivity
curl -X GET "http://localhost:9200/_cluster/health"

# Test your query directly
curl -X POST "http://localhost:9200/logs-*/_search" \
  -H "Content-Type: application/json" \
  -d '{"query":{"match_all":{}},"size":1}'

# Check time range
curl -X POST "http://localhost:9200/logs-*/_search" \
  -H "Content-Type: application/json" \
  -d '{"query":{"range":{"@timestamp":{"gte":"now-1h"}}},"size":1}'
```

**Solutions:**
- Verify Elasticsearch URL and credentials
- Check time range in `start_time` and `end_time`
- Validate JSON paths against actual Elasticsearch response
- Ensure cluster names match actual cluster names in data

### 2. Transformation Issues

**Symptoms:**
- Load debug shows unexpected data types or values
- Conversion functions not working

**Debug Steps:**
```yaml
# Add debug output to see before/after transformation
transform:
  stateless: false
  substitute_zeros_for_null: true
  conversion_functions:
    - field: response_time
      function: convert_type
      from_type: string
      to_type: float
```

**Solutions:**
- Check field names in conversion functions
- Verify data types in extract debug output
- Test conversion functions with sample data

### 3. Load Stream Failures

**Symptoms:**
- Metrics not appearing in Prometheus/OTEL
- Load errors in logs

**Debug Steps:**
```bash
# Test endpoints directly
curl -X POST "http://localhost:9091/metrics/job/elasticetl" \
  -H "Content-Type: text/plain" \
  -d "test_metric 123"

# Check OTEL collector
curl -X POST "http://localhost:4318/v1/metrics" \
  -H "Content-Type: application/json" \
  -d '{"test":"data"}'
```

**Solutions:**
- Verify endpoint URLs and authentication
- Check network connectivity
- Validate metric formats in debug output

### 4. Performance Issues

**Symptoms:**
- High memory usage
- Slow processing
- Timeouts

**Debug Steps:**
```bash
# Monitor resource usage
top -p $(pgrep elasticetl)

# Check metrics for performance data
curl http://localhost:8090/metrics | grep -E "(memory|cpu|duration)"

# Enable profiling (if built with pprof)
go tool pprof http://localhost:8090/debug/pprof/heap
```

**Solutions:**
- Adjust resource limits in configuration
- Reduce processing intervals
- Optimize Elasticsearch queries
- Limit concurrent operations

## Debug Output Analysis

### Extract Debug Analysis

```bash
# Count successful extractions
jq '.results | length' /tmp/elasticetl/debug/extract/*.json

# Check extracted values
jq '.results[].data' /tmp/elasticetl/debug/extract/*.json

# Verify cluster names
jq '.results[].metadata.cluster_name' /tmp/elasticetl/debug/extract/*.json

# Check query substitution
jq '.results[].metadata.query' /tmp/elasticetl/debug/extract/*.json
```

### Load Debug Analysis

```bash
# Check transformed data
jq '.results[].transformed_data' /tmp/elasticetl/debug/load/*.json

# Verify data types
jq '.results[].transformed_data | to_entries[] | {key: .key, type: (.value | type)}' /tmp/elasticetl/debug/load/*.json

# Check for null values
jq '.results[].transformed_data | to_entries[] | select(.value == null)' /tmp/elasticetl/debug/load/*.json
```

## Performance Debugging

### 1. Enable Detailed Metrics

```yaml
global:
  metrics:
    enabled: true
    port: 8090
    path: /metrics
    interval: 10s  # More frequent updates for debugging
```

### 2. Monitor Key Metrics

```bash
# Pipeline execution times
curl -s http://localhost:8090/metrics | grep pipeline_duration

# Memory usage
curl -s http://localhost:8090/metrics | grep memory

# Error rates
curl -s http://localhost:8090/metrics | grep error

# Data throughput
curl -s http://localhost:8090/metrics | grep -E "(entries|bytes)_processed"
```

### 3. Resource Optimization

```yaml
global:
  resource_limits:
    max_memory_mb: 256      # Reduce if memory constrained
    max_cpu_percent: 50     # Limit CPU usage
    max_goroutines: 50      # Reduce concurrency
    max_connections: 25     # Limit HTTP connections
```

## Debugging Checklist

### Before Running
- [ ] Configuration syntax is valid (JSON/YAML)
- [ ] Elasticsearch endpoints are accessible
- [ ] Output destinations (Prometheus, OTEL) are reachable
- [ ] Debug directories have write permissions
- [ ] Time expressions are valid

### During Execution
- [ ] Extract debug files are being created
- [ ] Load debug files show expected transformations
- [ ] Metrics endpoint is responding
- [ ] No error messages in logs
- [ ] Resource usage is within limits

### After Issues
- [ ] Check debug output files for data flow
- [ ] Verify Elasticsearch query results manually
- [ ] Test output destinations independently
- [ ] Review configuration for typos or incorrect values
- [ ] Check system resources and network connectivity

## Advanced Debugging

### 1. Custom Debug Streams

Create multiple debug streams for different purposes:

```yaml
load:
  streams:
    - type: debug
      config:
        path: /tmp/elasticetl/debug/raw_data
    - type: debug  
      config:
        path: /tmp/elasticetl/debug/processed_data
    - type: prometheus
      config:
        endpoint: http://localhost:9091/metrics/job/elasticetl
```

### 2. Conditional Debugging

Enable debugging only for specific conditions:

```yaml
extract:
  debug:
    enabled: true
    path: /tmp/elasticetl/debug/extract
  # Only debug when there are issues
  max_retries: 0  # Fail fast for debugging
```

### 3. Pipeline-Specific Debugging

Debug individual pipelines:

```bash
# Run only specific pipeline
./elasticetl -config configs/config.yaml -pipeline my-pipeline -log-level debug
```

This comprehensive debugging guide should help you troubleshoot any issues with ElasticETL pipelines and understand the data flow through the system.
