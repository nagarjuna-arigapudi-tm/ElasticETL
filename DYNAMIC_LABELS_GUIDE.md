# Dynamic Labels Guide

This guide explains how to use the dynamic labeling feature in ElasticETL, where flattened JSON keys become CSV column headers, and the values in those columns are used as label values for time-series databases.

## Overview

The dynamic labeling system works in three stages:

1. **JSON Flattening**: Nested JSON from Elasticsearch is flattened into key-value pairs
2. **CSV Generation**: Flattened keys become CSV column headers, values become row data
3. **Dynamic Labeling**: CSV column headers are mapped to label names, row values become label values

## Configuration Structure

```yaml
load:
  streams:
    - name: "prometheus-stream"
      type: "prometheus"
      config:
        remote_write_url: "http://prometheus:9090/api/v1/write"
        
        # Dynamic Labels Configuration
        dynamic_labels:
          - label_name: "cluster"        # Label name in TSDB
            csv_column: "key"            # CSV column header to get value from
          - label_name: "environment"
            static_value: "production"   # Static value (not from CSV)
        
        # Metric Columns Configuration  
        metric_columns:
          - column: "node_count.value"   # CSV column containing metric value
            metric_name: "cluster_nodes" # Metric name in TSDB
```

## Example Workflow

### 1. Elasticsearch Response (Nested JSON)
```json
{
  "aggregations": {
    "clusters": {
      "buckets": [
        {
          "key": "prod-cluster-1",
          "status": {
            "buckets": [{"key": "green"}]
          },
          "node_count": {"value": 5},
          "avg_cpu": {"value": 45.2}
        }
      ]
    }
  }
}
```

### 2. After JSON Flattening
```json
{
  "key": "prod-cluster-1",
  "status.buckets.0.key": "green", 
  "node_count.value": 5,
  "avg_cpu.value": 45.2
}
```

### 3. CSV Generation
```csv
key,status.buckets.0.key,node_count.value,avg_cpu.value
prod-cluster-1,green,5,45.2
```

### 4. Dynamic Label Mapping
With this configuration:
```yaml
dynamic_labels:
  - label_name: "cluster_name"
    csv_column: "key"
  - label_name: "health_status"  
    csv_column: "status.buckets.0.key"
  - label_name: "environment"
    static_value: "production"

metric_columns:
  - column: "node_count.value"
    metric_name: "cluster_nodes_total"
  - column: "avg_cpu.value"
    metric_name: "cluster_cpu_percent"
```

### 5. Final Prometheus Metrics
```
cluster_nodes_total{cluster_name="prod-cluster-1",health_status="green",environment="production"} 5
cluster_cpu_percent{cluster_name="prod-cluster-1",health_status="green",environment="production"} 45.2
```

## Configuration Options

### Dynamic Labels

#### CSV Column Labels
```yaml
dynamic_labels:
  - label_name: "cluster"      # Label name in target system
    csv_column: "key"          # CSV column header to read value from
```

#### Static Labels  
```yaml
dynamic_labels:
  - label_name: "job"
    static_value: "etl-job"    # Fixed value for all metrics
```

### Metric Columns

```yaml
metric_columns:
  - column: "response_time.value"     # CSV column containing numeric value
    metric_name: "http_response_ms"   # Metric name in target system
```

## Filtering Integration

Use filters to control which flattened keys become CSV columns:

```yaml
extract:
  filters:
    - type: "include"
      pattern: "key"                    # Include cluster name
    - type: "include"  
      pattern: "*.buckets.0.key"        # Include first bucket key from any aggregation
    - type: "include"
      pattern: "*.value"                # Include all metric values
    - type: "exclude"
      pattern: "*.doc_count"            # Exclude document counts
```

## Multiple Streams Example

```yaml
load:
  streams:
    # Prometheus with cluster-level labels
    - name: "prometheus-cluster"
      type: "prometheus" 
      config:
        dynamic_labels:
          - label_name: "cluster"
            csv_column: "cluster_name"
          - label_name: "datacenter"
            csv_column: "dc_name"
        metric_columns:
          - column: "total_nodes"
            metric_name: "cluster_nodes"
    
    # OTEL with different labeling
    - name: "otel-collector"
      type: "otel"
      config:
        dynamic_labels:
          - label_name: "service.name"
            static_value: "elasticsearch"
          - label_name: "cluster.id"
            csv_column: "cluster_name"
        resource_attributes:
          - attribute: "cluster.environment"
            csv_column: "environment"
```

## Best Practices

1. **Use Descriptive Label Names**: Choose clear, consistent label names
2. **Limit Label Cardinality**: Avoid high-cardinality labels (many unique values)
3. **Filter Appropriately**: Use filters to include only necessary data
4. **Static Labels for Context**: Use static labels for environment, job, etc.
5. **Validate CSV Columns**: Ensure referenced CSV columns exist in flattened data

## Troubleshooting

### Missing CSV Columns
If a referenced CSV column doesn't exist:
- Check your JSON path extraction
- Verify your filters include the required keys
- Review the flattened JSON structure

### High Cardinality Labels
If you have too many unique label values:
- Use filters to limit the data
- Consider aggregating at a higher level
- Use static labels where possible

### Label Name Conflicts
If label names conflict:
- Use unique, descriptive names
- Follow your monitoring system's naming conventions
- Consider prefixing labels by source (e.g., "es_cluster", "es_node")

## Real-World Examples

### Example 1: Cluster Health Monitoring
```yaml
# Extract cluster health data
extract:
  json_path: "$.aggregations.clusters.buckets"
  filters:
    - type: "include"
      pattern: "key"                    # cluster name
    - type: "include"
      pattern: "status.buckets.0.key"   # health status
    - type: "include"
      pattern: "node_count.value"       # node count

# Create labels from CSV data
load:
  streams:
    - type: "prometheus"
      config:
        dynamic_labels:
          - label_name: "cluster_name"
            csv_column: "key"
          - label_name: "health_status"
            csv_column: "status.buckets.0.key"
        metric_columns:
          - column: "node_count.value"
            metric_name: "elasticsearch_cluster_nodes"
```

### Example 2: Node Performance Metrics
```yaml
# Extract node-level performance data
extract:
  json_path: "$.aggregations.nodes.buckets"
  filters:
    - type: "include"
      pattern: "key"                    # node name
    - type: "include"
      pattern: "cluster.buckets.0.key"  # cluster name
    - type: "include"
      pattern: "cpu_usage.value"        # CPU usage
    - type: "include"
      pattern: "memory_usage.value"     # Memory usage

# Create detailed labels
load:
  streams:
    - type: "prometheus"
      config:
        dynamic_labels:
          - label_name: "node_name"
            csv_column: "key"
          - label_name: "cluster_name"
            csv_column: "cluster.buckets.0.key"
          - label_name: "datacenter"
            static_value: "dc1"
        metric_columns:
          - column: "cpu_usage.value"
            metric_name: "elasticsearch_node_cpu_percent"
          - column: "memory_usage.value"
            metric_name: "elasticsearch_node_memory_percent"
```

This dynamic labeling system provides flexible, data-driven labeling that automatically adapts to your Elasticsearch data structure while maintaining consistent metric naming and labeling conventions.
