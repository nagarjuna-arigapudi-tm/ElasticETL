# Prometheus Metrics Guide - CSV-Based Time Series

This guide explains how to use ElasticETL's enhanced Prometheus metrics functionality that creates time series from CSV data using the `metrics` configuration.

## Overview

ElasticETL can now generate Prometheus time series directly from CSV data by grouping rows based on unique field combinations and creating separate time series for each unique group. This is particularly useful for monitoring metrics from multiple sources (like load balancers, servers, etc.) where each source should have its own time series.

## Configuration Format

The metrics configuration is added to the stream configuration under the `config` section:

```yaml
streams:
  - type: "debug"  # or "gem" for remote write
    config:
      # ... other config ...
      metrics:
        - name: "metric_name"
          uniquefieldsIndex: [0, 1]  # Columns to group by
          value: 2                   # Column containing the metric value
          timestamp: 3               # Column containing the timestamp
          labels:
            - label_name: "dynamic_label"
              index_in_csv_data: 0    # Use value from CSV column
            - label_name: "static_label"
              static_value: "constant_value"
```

## Configuration Fields

### Metric Configuration

- **`name`**: The name of the Prometheus metric (becomes `__name__` label)
- **`uniquefieldsIndex`**: Array of column indices used to group CSV rows into separate time series
- **`value`**: Column index containing the numeric metric value
- **`timestamp`**: Column index containing the timestamp (Unix timestamp in seconds or milliseconds)
- **`labels`**: Array of label configurations for the time series

### Label Configuration

- **`label_name`**: The name of the Prometheus label
- **`index_in_csv_data`**: Column index to get the label value from (for dynamic labels)
- **`static_value`**: Fixed value for the label (for static labels)

## Example Scenario

Given CSV data like this:
```
alb1,712223444,23.4,7802
alb1,713223444,25.4,7812
alb1,714223444,20.4,7822
alb1,715223444,34.4,7832
alb2,716223444,21.4,7842
alb2,717223444,16.4,7852
```

With this metrics configuration:
```yaml
metrics:
  - name: "cpuusage"
    uniquefieldsIndex: [0]  # Group by column 0 (load balancer name)
    value: 2                # CPU usage is in column 2
    timestamp: 1            # Timestamp is in column 1
    labels:
      - label_name: "LB_Name"
        index_in_csv_data: 0  # Load balancer name from column 0
      - label_name: "job"
        static_value: "elasticsearch-etl"
```

This generates two time series:

**Time Series 1 (alb1):**
```
timeseries {
  labels: { __name__="cpuusage", LB_Name="alb1", job="elasticsearch-etl" }
  samples: { timestamp: 712223444, value: 23.4 }
  samples: { timestamp: 713223444, value: 25.4 }
  samples: { timestamp: 714223444, value: 20.4 }
  samples: { timestamp: 715223444, value: 34.4 }
}
```

**Time Series 2 (alb2):**
```
timeseries {
  labels: { __name__="cpuusage", LB_Name="alb2", job="elasticsearch-etl" }
  samples: { timestamp: 716223444, value: 21.4 }
  samples: { timestamp: 717223444, value: 16.4 }
}
```

## Multiple Metrics

You can define multiple metrics from the same CSV data:

```yaml
metrics:
  - name: "cpuusage"
    uniquefieldsIndex: [0]
    value: 2
    timestamp: 1
    labels:
      - label_name: "LB_Name"
        index_in_csv_data: 0
      - label_name: "job"
        static_value: "elasticsearch-etl"
        
  - name: "requestsProcessed"
    uniquefieldsIndex: [0]
    value: 3
    timestamp: 1
    labels:
      - label_name: "LoadBalancer"
        index_in_csv_data: 0
      - label_name: "job"
        static_value: "elasticsearch-etl"
```

## Stream Types Supporting Metrics

### Debug Stream
Use `format: "prometheus"` to see the generated time series in debug files:

```yaml
- type: "debug"
  config:
    path: "/tmp/elasticetl/debug/metrics"
    format: "prometheus"
    metrics:
      # ... metrics configuration ...
```

### GEM Stream (Prometheus Remote Write)
For sending to Prometheus via remote write API:

```yaml
- type: "gem"
  config:
    endpoint: "https://prometheus.example.com/api/v1/write"
    metrics:
      # ... metrics configuration ...
  basic_auth:
    username: "${PROMETHEUS_USER}"
    password: "${PROMETHEUS_PASSWORD}"
```

## Complex Grouping

You can group by multiple columns for more complex scenarios:

```yaml
metrics:
  - name: "server_cpu"
    uniquefieldsIndex: [0, 1]  # Group by server AND region
    value: 3
    timestamp: 2
    labels:
      - label_name: "server"
        index_in_csv_data: 0
      - label_name: "region"
        index_in_csv_data: 1
      - label_name: "metric_type"
        static_value: "cpu_utilization"
```

This creates separate time series for each unique combination of server and region.

## Data Requirements

1. **CSV Format**: The transform phase must output CSV format (`output_format: "csv"`)
2. **Numeric Values**: The value column must contain numeric data (use conversion functions if needed)
3. **Timestamps**: Timestamps should be Unix timestamps (seconds or milliseconds)
4. **Consistent Columns**: All CSV rows should have the same number of columns

## Transform Configuration

Ensure your transform configuration outputs CSV and converts data types as needed:

```yaml
transform:
  stateless: true
  substitute_zeros_for_null: true
  output_format: "csv"  # Required for metrics functionality
  conversion_functions:
    - field: "_source.cpu_usage"
      function: "convert_type"
      from_type: "string"
      to_type: "float"
    - field: "_source.timestamp"
      function: "convert_type"
      from_type: "string"
      to_type: "int"
```

## Troubleshooting

### No Time Series Generated
- Check that CSV data is being generated (use debug stream with JSON format first)
- Verify column indices are correct (0-based indexing)
- Ensure value and timestamp columns contain valid numeric data

### Missing Labels
- Verify `index_in_csv_data` values are within the CSV column range
- Check that CSV rows have consistent column counts

### Timestamp Issues
- Timestamps should be Unix timestamps (seconds or milliseconds)
- Use conversion functions to convert string timestamps to integers

## Complete Example

See `examples/prometheus-metrics-config.yaml` for a complete working configuration that demonstrates:
- Elasticsearch data extraction
- CSV transformation with type conversion
- Multiple metrics from the same data
- Both debug output and Prometheus remote write
- Environment variable usage for authentication

This configuration creates production-ready Prometheus metrics from Elasticsearch data with proper labeling and grouping.
