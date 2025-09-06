# Nested Array CSV Transformation Guide

This guide explains how ElasticETL handles nested array structures when converting flattened JSON data to CSV format.

## Overview

When Elasticsearch aggregation results contain nested arrays (like buckets within buckets), ElasticETL can automatically expand these into multiple CSV rows, creating a denormalized table structure that's perfect for analysis tools.

## How It Works

### Input Data Structure (Flattened JSON)

Consider this flattened Elasticsearch aggregation result:

```
[0].key = "api-service"
[0].doc_count = 1000
[0].avg_response_time = 125.5
[0].hosts.buckets[0].key = "host-1"
[0].hosts.buckets[0].cpu_usage.buckets[0].system = 15.7
[0].hosts.buckets[0].cpu_usage.buckets[0].user = 55.2
[0].hosts.buckets[0].cpu_usage.buckets[0].idle = 57.3
[0].hosts.buckets[0].cpu_usage.buckets[1].system = 25.7
[0].hosts.buckets[0].cpu_usage.buckets[1].user = 53.1
[0].hosts.buckets[0].cpu_usage.buckets[1].idle = 25.2
[0].hosts.buckets[1].key = "host-2"
[0].hosts.buckets[1].cpu_usage.buckets[0].system = 35.7
[0].hosts.buckets[1].cpu_usage.buckets[0].user = 52.2
[0].hosts.buckets[1].cpu_usage.buckets[0].idle = 34.2
[0].hosts.buckets[1].cpu_usage.buckets[1].system = 15.7
[0].hosts.buckets[1].cpu_usage.buckets[1].user = 23.1
[0].hosts.buckets[1].cpu_usage.buckets[1].idle = 60.2
[1].key = "web-service"
[1].doc_count = 500
[1].avg_response_time = 89.3
[1].hosts.buckets[0].key = "host-3"
[1].hosts.buckets[0].cpu_usage.buckets[0].system = 6.7
[1].hosts.buckets[0].cpu_usage.buckets[0].user = 9.3
[1].hosts.buckets[0].cpu_usage.buckets[0].idle = 74.6
[1].hosts.buckets[0].cpu_usage.buckets[1].system = 27.0
[1].hosts.buckets[0].cpu_usage.buckets[1].user = 13.2
[1].hosts.buckets[0].cpu_usage.buckets[1].idle = 63.5
[1].hosts.buckets[1].key = "host-4"
[1].hosts.buckets[1].cpu_usage.buckets[0].system = 20.7
[1].hosts.buckets[1].cpu_usage.buckets[0].user = 12.3
[1].hosts.buckets[1].cpu_usage.buckets[0].idle = 54.5
[1].hosts.buckets[1].cpu_usage.buckets[1].system = 27.7
[1].hosts.buckets[1].cpu_usage.buckets[1].user = 33.4
[1].hosts.buckets[1].cpu_usage.buckets[1].idle = 40.9
```

### Depth-Based Unique Key Analysis

The transformer analyzes flattened keys by depth levels:

**Level 2 (after removing array indices):**
- `key`, `doc_count`, `avg_response_time`

**Level 4:**
- `hosts.buckets.key`

**Level 6:**
- `hosts.buckets.cpu_usage.buckets.system`
- `hosts.buckets.cpu_usage.buckets.user`
- `hosts.buckets.cpu_usage.buckets.idle`

**Final Unique Keys:**
`key,doc_count,avg_response_time,hosts.buckets.key,hosts.buckets.cpu_usage.buckets.system,hosts.buckets.cpu_usage.buckets.user,hosts.buckets.cpu_usage.buckets.idle`

### Output CSV Structure

The transformer creates multiple rows by expanding all array combinations:

```csv
key,doc_count,avg_response_time,hosts.buckets.key,hosts.buckets.cpu_usage.buckets.system,hosts.buckets.cpu_usage.buckets.user,hosts.buckets.cpu_usage.buckets.idle
"api-service",1000,125.5,"host-1",15.7,55.2,57.3
"api-service",1000,125.5,"host-1",25.7,53.1,25.2
"api-service",1000,125.5,"host-2",35.7,52.2,34.2
"api-service",1000,125.5,"host-2",15.7,23.1,60.2
"web-service",500,89.3,"host-3",6.7,9.3,74.6
"web-service",500,89.3,"host-3",27.0,13.2,63.5
"web-service",500,89.3,"host-4",20.7,12.3,54.5
"web-service",500,89.3,"host-4",27.7,33.4,40.9
```

## Key Features

### 1. Automatic Nested Array Detection

The transformer automatically identifies nested arrays that contain objects:

- Arrays of objects are expanded into multiple rows
- Simple arrays (strings, numbers) are handled as regular values
- Parent data is repeated for each nested array item

### 2. Flattened Column Names

Nested object properties become flattened column names:

- `hosts[0].key` becomes `hosts.key`
- `hosts[0].cpu_usage` becomes `hosts.cpu_usage`
- `metrics.memory.usage` becomes `metrics.memory.usage`

### 3. Multiple Nested Arrays

If multiple nested arrays exist at the same level, the transformer creates a Cartesian product:

```json
{
  "service": "api",
  "hosts": [{"name": "host1"}, {"name": "host2"}],
  "metrics": [{"type": "cpu"}, {"type": "memory"}]
}
```

Results in 4 rows (2 hosts × 2 metrics):
```csv
service,hosts.name,metrics.type
"api","host1","cpu"
"api","host1","memory"
"api","host2","cpu"
"api","host2","memory"
```

### 4. Data Repetition

Parent-level data is automatically repeated for each nested array combination:

- Service-level metrics appear in every row for that service
- Cluster-level information is repeated across all hosts
- Timestamp and metadata are preserved in each row

## Configuration

### Enable CSV Output

Set the output format in your transform configuration:

```yaml
transform:
  output_format: "csv"  # Enable CSV transformation
  stateless: true
  substitute_zeros_for_null: true
```

### CSV Stream Configuration

Configure a CSV load stream to save the results:

```yaml
load:
  streams:
    - type: "csv"
      config:
        path: "/path/to/output/directory"
      labels:
        pipeline: "my-pipeline"
```

## Example Configuration

Here's a complete configuration for nested array CSV transformation:

```yaml
pipelines:
  - name: "service-host-metrics"
    enabled: true
    interval: "5m"
    
    extract:
      elasticsearch_query: |
        {
          "size": 0,
          "aggs": {
            "services": {
              "terms": {"field": "service.keyword"},
              "aggs": {
                "avg_response_time": {"avg": {"field": "response_time"}},
                "hosts": {
                  "terms": {"field": "host.keyword"},
                  "aggs": {
                    "cpu_usage": {"avg": {"field": "cpu_usage"}},
                    "memory_usage": {"avg": {"field": "memory_usage"}}
                  }
                }
              }
            }
          }
        }
      urls: ["https://elasticsearch:9200/metrics-*/_search"]
      cluster_names: ["production"]
      json_path: "aggregations.services.buckets"
      
    transform:
      output_format: "csv"
      stateless: true
      substitute_zeros_for_null: true
      
    load:
      streams:
        - type: "csv"
          config:
            path: "/data/metrics/service-host-metrics"
```

## Use Cases

### 1. Service Performance Analysis

Transform Elasticsearch service aggregations into CSV for:
- Excel analysis
- Business intelligence tools
- Data visualization platforms
- Statistical analysis

### 2. Infrastructure Monitoring

Convert host and container metrics into tabular format for:
- Capacity planning
- Performance trending
- Resource utilization analysis
- Cost optimization

### 3. Application Metrics

Transform application performance data for:
- SLA reporting
- Performance dashboards
- Alerting thresholds
- Trend analysis

## Best Practices

### 1. Column Ordering

Columns are automatically sorted alphabetically. For custom ordering, consider:
- Using prefixes in your field names
- Post-processing the CSV files
- Using specific field naming conventions

### 2. Memory Considerations

Large nested arrays can create many CSV rows:
- Monitor memory usage with large datasets
- Consider pagination in Elasticsearch queries
- Use appropriate resource limits

### 3. Data Types

The transformer handles various data types:
- Numbers are formatted without scientific notation
- Strings are properly quoted
- Booleans become "true"/"false"
- Null values become empty strings

### 4. File Management

CSV files are timestamped automatically:
- Files include timestamp in filename
- Multiple pipelines can write to same directory
- Consider log rotation for long-running pipelines

## Troubleshooting

### Empty CSV Files

If CSV files are empty:
- Check that `output_format: "csv"` is set
- Verify the JSON path extracts data correctly
- Ensure nested arrays contain objects

### Missing Columns

If expected columns are missing:
- Verify field names in the source data
- Check for typos in aggregation names
- Ensure data exists in the time range

### Too Many Rows

If CSV files are too large:
- Reduce the size of nested arrays in queries
- Add filters to limit data volume
- Consider splitting into multiple pipelines

## Advanced Features

### Custom Field Transformations

Apply transformations before CSV conversion:

```yaml
transform:
  output_format: "csv"
  conversion_functions:
    - field: "avg_response_time.value"
      function: "convert_to_mb"
      from_unit: "bytes"
    - field: "cpu_usage.value"
      function: "convert_type"
      from_type: "float"
      to_type: "int"
```

### Multiple Output Formats

Generate both CSV and other formats:

```yaml
load:
  streams:
    - type: "csv"
      config:
        path: "/data/csv-output"
    - type: "prometheus"
      config:
        endpoint: "http://prometheus:9090/api/v1/write"
    - type: "debug"
      config:
        path: "/data/debug-output"
```

This allows you to maintain both structured CSV files and real-time metrics simultaneously.
