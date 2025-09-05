# ElasticETL CSV Flattening and Labeling Guide

This guide explains the new features in ElasticETL for JSON flattening, CSV output, and advanced labeling capabilities.

## Overview

ElasticETL now supports:
1. **Single JSON Path Extraction**: Extract a specific part of the Elasticsearch response
2. **JSON Flattening**: Automatically flatten nested JSON structures into CSV-compatible format
3. **Filtering**: Exclude specific flattened keys from the output
4. **CSV Output**: Export data in CSV format with proper handling of nested arrays
5. **Label Columns**: Use CSV columns as labels when loading to time-series databases

## Key Concepts

### JSON Flattening

JSON flattening converts nested JSON structures into flat key-value pairs suitable for CSV export:

**Original JSON:**
```json
{
  "services": {
    "buckets": [
      {
        "key": "api-service",
        "doc_count": 1000,
        "avg_response_time": {
          "value": 125.5
        },
        "hosts": {
          "buckets": [
            {
              "key": "host-1",
              "cpu_usage": {
                "value": 75.2
              }
            }
          ]
        }
      }
    ]
  }
}
```

**Flattened Keys:**
```
services.buckets[0].key = "api-service"
services.buckets[0].doc_count = 1000
services.buckets[0].avg_response_time.value = 125.5
services.buckets[0].hosts.buckets[0].key = "host-1"
services.buckets[0].hosts.buckets[0].cpu_usage.value = 75.2
```

### Special "value" Key Handling

When a JSON object has a single key named "value" (case-insensitive), the value is assigned to the parent key:

**Before:**
```json
{
  "avg_response_time": {
    "value": 125.5
  }
}
```

**After Flattening:**
```
avg_response_time = 125.5
```

### CSV Row Generation

When arrays are present in the flattened data, multiple CSV rows are created:
- Outer-level values are repeated for each array element
- Each array element creates a new row
- Missing values are filled with empty strings

## Configuration

### Extract Configuration

```yaml
extract:
  # Single JSON path to extract and flatten
  json_path: aggregations.services.buckets
  
  # Filters to exclude flattened keys
  filters:
    - key: "*.doc_count_error_upper_bound"
      exclude: true
    - key: "*[*].hosts.buckets[*].doc_count"
      exclude: true
```

#### JSON Path
- **Purpose**: Specifies which part of the Elasticsearch response to extract
- **Format**: JSONPath expression (e.g., `aggregations.services.buckets`)
- **Behavior**: The extracted JSON is then flattened into key-value pairs

#### Filters
- **Purpose**: Exclude specific flattened keys from the output
- **Key Patterns**: Support wildcards (`*`) for pattern matching
- **Exclude**: When `true`, matching keys are removed from the output

### Transform Configuration

```yaml
transform:
  output_format: csv  # Enable CSV output format
  
  # Apply transformations to flattened field paths
  conversion_functions:
    - field: "[*].avg_response_time.value"
      function: convert_type
      from_type: string
      to_type: float
```

#### Output Format
- **csv**: Enables CSV output format with flattened data
- **json**: Traditional JSON format (default)

#### Conversion Functions
- **Field**: Use flattened field paths (e.g., `[*].avg_response_time.value`)
- **Wildcards**: `[*]` matches any array index
- **Functions**: Same as before (convert_type, convert_to_kb, etc.)

### Load Configuration

```yaml
load:
  # Columns to use as labels when loading to TSDB
  label_columns:
    - "[*].key"  # service name
    - "[*].hosts.buckets[*].key"  # host name
  
  streams:
    - type: csv
      config:
        path: /tmp/elasticetl/output/services_metrics
```

#### Label Columns
- **Purpose**: Specify which CSV columns should be used as labels
- **Format**: Flattened field paths
- **Usage**: When loading to Prometheus/OTEL/GEM, these columns become labels

#### CSV Stream
- **Type**: `csv`
- **Path**: Base path for CSV files (timestamp will be appended)
- **Output**: Creates timestamped CSV files with headers and data rows

## Example Workflow

### 1. Elasticsearch Query Result

```json
{
  "aggregations": {
    "services": {
      "buckets": [
        {
          "key": "api-service",
          "doc_count": 1000,
          "avg_response_time": {
            "value": 125.5
          },
          "hosts": {
            "buckets": [
              {
                "key": "host-1",
                "cpu_usage": {
                  "value": 75.2
                }
              },
              {
                "key": "host-2",
                "cpu_usage": {
                  "value": 68.9
                }
              }
            ]
          }
        },
        {
          "key": "web-service",
          "doc_count": 500,
          "avg_response_time": {
            "value": 89.3
          },
          "hosts": {
            "buckets": [
              {
                "key": "host-3",
                "cpu_usage": {
                  "value": 82.1
                }
              }
            ]
          }
        }
      ]
    }
  }
}
```

### 2. JSON Path Extraction

With `json_path: aggregations.services.buckets`, we extract:

```json
[
  {
    "key": "api-service",
    "doc_count": 1000,
    "avg_response_time": {
      "value": 125.5
    },
    "hosts": {
      "buckets": [
        {
          "key": "host-1",
          "cpu_usage": {
            "value": 75.2
          }
        },
        {
          "key": "host-2",
          "cpu_usage": {
            "value": 68.9
          }
        }
      ]
    }
  },
  {
    "key": "web-service",
    "doc_count": 500,
    "avg_response_time": {
      "value": 89.3
    },
    "hosts": {
      "buckets": [
        {
          "key": "host-3",
          "cpu_usage": {
            "value": 82.1
          }
        }
      ]
    }
  }
]
```

### 3. Flattening

The extracted JSON is flattened to:

```
[0].key = "api-service"
[0].doc_count = 1000
[0].avg_response_time = 125.5
[0].hosts.buckets[0].key = "host-1"
[0].hosts.buckets[0].cpu_usage = 75.2
[0].hosts.buckets[1].key = "host-2"
[0].hosts.buckets[1].cpu_usage = 68.9
[1].key = "web-service"
[1].doc_count = 500
[1].avg_response_time = 89.3
[1].hosts.buckets[0].key = "host-3"
[1].hosts.buckets[0].cpu_usage = 82.1
```

### 4. CSV Output

The flattened data becomes CSV:

```csv
[0].key,[0].doc_count,[0].avg_response_time,[0].hosts.buckets[0].key,[0].hosts.buckets[0].cpu_usage,[0].hosts.buckets[1].key,[0].hosts.buckets[1].cpu_usage,[1].key,[1].doc_count,[1].avg_response_time,[1].hosts.buckets[0].key,[1].hosts.buckets[0].cpu_usage
api-service,1000,125.5,host-1,75.2,host-2,68.9,,,,,
,,,,,,,web-service,500,89.3,host-3,82.1
```

### 5. Label-Based Loading

With `label_columns: ["[*].key", "[*].hosts.buckets[*].key"]`, when loading to Prometheus:

```
service_metric{service="api-service", host="host-1", environment="production"} 125.5
service_metric{service="api-service", host="host-2", environment="production"} 125.5
service_metric{service="web-service", host="host-3", environment="production"} 89.3
```

## Advanced Features

### Filtering Examples

```yaml
filters:
  # Exclude all doc_count fields
  - key: "*.doc_count"
    exclude: true
  
  # Exclude nested doc_count fields in arrays
  - key: "*[*].doc_count"
    exclude: true
  
  # Exclude specific nested paths
  - key: "*[*].hosts.buckets[*].doc_count"
    exclude: true
  
  # Exclude error bounds
  - key: "*.doc_count_error_upper_bound"
    exclude: true
```

### Transformation Examples

```yaml
conversion_functions:
  # Convert response times to float
  - field: "[*].avg_response_time"
    function: convert_type
    from_type: string
    to_type: float
  
  # Convert CPU usage in nested arrays
  - field: "[*].hosts.buckets[*].cpu_usage"
    function: convert_type
    from_type: string
    to_type: float
  
  # Convert memory from bytes to MB
  - field: "[*].hosts.buckets[*].memory_bytes"
    function: convert_to_mb
    from_unit: bytes
```

### Multiple Output Streams

```yaml
load:
  label_columns:
    - "[*].key"
    - "[*].hosts.buckets[*].key"
  
  streams:
    # CSV for analysis
    - type: csv
      config:
        path: /tmp/elasticetl/output/services
    
    # Debug for troubleshooting
    - type: debug
      config:
        path: /tmp/elasticetl/debug/load
    
    # Prometheus with labels
    - type: prometheus
      config:
        endpoint: http://localhost:9091/metrics/job/services
      labels:
        environment: production
        team: platform
    
    # OTEL with labels
    - type: otel
      config:
        endpoint: http://localhost:4318/v1/metrics
      labels:
        service: elasticetl
        version: "2.0"
```

## Best Practices

### 1. JSON Path Selection
- Choose the most specific path that contains your data
- Avoid extracting the entire response if you only need a subset
- Test your JSON path with sample data first

### 2. Filtering Strategy
- Filter out metadata fields that aren't needed for analysis
- Remove error bounds and statistical metadata
- Keep only the metrics and dimensions you need

### 3. Label Column Selection
- Choose columns that provide meaningful dimensions
- Avoid high-cardinality labels (too many unique values)
- Include cluster names and service identifiers

### 4. Transformation Planning
- Apply type conversions early in the pipeline
- Use unit conversions for consistency
- Handle null values appropriately

### 5. Output Organization
- Use descriptive paths for CSV files
- Enable debug streams during development
- Monitor resource usage with complex flattening

## Troubleshooting

### Common Issues

1. **Empty CSV Output**
   - Check if JSON path exists in the response
   - Verify filters aren't excluding all data
   - Enable extract debug to see raw data

2. **Incorrect Flattening**
   - Review the JSON structure in debug output
   - Check for unexpected nesting levels
   - Verify "value" key handling

3. **Missing Labels**
   - Ensure label columns exist in flattened data
   - Check column name patterns match actual keys
   - Verify label column configuration

4. **Performance Issues**
   - Limit the size of extracted JSON
   - Use filters to reduce data volume
   - Monitor memory usage with large datasets

### Debug Commands

```bash
# View extract debug output
cat /tmp/elasticetl/debug/extract/*.json | jq .

# View flattened structure
cat /tmp/elasticetl/debug/load/*.json | jq '.results[].transformed_data'

# Check CSV headers
head -1 /tmp/elasticetl/output/*.csv

# Count CSV rows
wc -l /tmp/elasticetl/output/*.csv
```

This new flattening and CSV capability makes ElasticETL much more powerful for handling complex Elasticsearch aggregations and exporting them in formats suitable for analysis and time-series databases.
