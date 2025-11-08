# CSV Format Support Guide

This guide explains how to use the new CSV format support in ElasticETL, which allows you to extract, transform, and load CSV data alongside the existing JSON functionality.

## Overview

ElasticETL now supports CSV format throughout the entire ETL pipeline:
- **Extract**: APIs can return CSV data instead of JSON
- **Transform**: Process CSV data with column-based transformations
- **Load**: Output CSV data to various destinations

## Configuration

### Extract Configuration

To extract CSV data, set the `output_format` field in the extract configuration:

```yaml
extract:
  output_format: "csv"  # "json" (default) or "csv"
  urls:
    - "http://api.example.com/data.csv"
  # Note: json_path and filters are not applicable for CSV format
```

**Important Notes:**
- When `output_format` is "csv", the `json_path` and `filters` parameters are ignored
- CSV data is stored in the `CSVData` field of the Result struct instead of the `Data` field
- The extractor automatically parses CSV format with proper quote handling

### Transform Configuration

Configure the transformer to handle CSV input:

```yaml
transform:
  input:
    format: "csv"     # "json" (default) or "csv"
    header: true      # true if CSV data has header row
  conversion_functions:
    - field_index: 0  # Use field_index for CSV columns instead of field
      function: "convert_type"
      from_type: "string"
      to_type: "float64"
    - field_index: 2
      function: "convert_to_mb"
      from_unit: "bytes"
      to_unit: "mb"
```

**Key Differences for CSV:**
- Use `field_index` instead of `field` to specify columns (0-based indexing)
- Set `input.format` to "csv" to enable CSV processing mode
- Use `input.header` to indicate if the first row contains column headers

### Load Configuration

Load CSV data to various destinations:

```yaml
load:
  input: "csv_data"  # Use "csv_data" for CSV format, "transformed_data" for JSON
  streams:
    - type: "csv"
      config:
        file_path: "/tmp/output.csv"
        include_timestamp: true
    - type: "prometheus"
      config:
        remote_write_url: "http://prometheus:9090/api/v1/write"
      metrics:
        - name: "system_memory_usage"
          uniquefieldsIndex: [0, 1]  # CSV column indices for unique identification
          value: 2                   # CSV column index for metric value
          timestamp: 3               # CSV column index for timestamp
```

## Data Flow

### CSV Processing Pipeline

1. **Extract Phase**:
   - API returns CSV data
   - Extractor parses CSV into `[][]string` format
   - Data stored in `Result.CSVData` field

2. **Transform Phase**:
   - Transformer processes CSV rows
   - Column-based transformations using `field_index`
   - Null value handling (drop entire rows if configured)
   - Output can be CSV or JSON format

3. **Load Phase**:
   - CSV data loaded to configured destinations
   - Support for Prometheus metrics, file output, debug streams

### Backward Compatibility

The CSV support is fully backward compatible:
- Existing JSON configurations continue to work unchanged
- Default `output_format` is "json" if not specified
- JSON and CSV pipelines can run simultaneously

## Example Configurations

### Basic CSV Pipeline

```yaml
pipelines:
  - name: "csv-metrics-pipeline"
    enabled: true
    schedule:
      interval: "1m"
    extract:
      output_format: "csv"
      urls:
        - "http://metrics-api/system-stats.csv"
      cluster_names:
        - "production"
      timeout: "30s"
    transform:
      input:
        format: "csv"
        header: true
      conversion_functions:
        - field_index: 1
          function: "convert_to_mb"
          from_unit: "bytes"
          to_unit: "mb"
    load:
      input: "csv_data"
      streams:
        - type: "csv"
          config:
            file_path: "/data/processed-metrics.csv"
```

### Mixed JSON and CSV Pipeline

```yaml
pipelines:
  - name: "json-pipeline"
    extract:
      output_format: "json"  # Default JSON processing
      json_path: "hits.hits._source"
      filters:
        - type: "include"
          pattern: "system\\..*"
    # ... rest of JSON configuration

  - name: "csv-pipeline"
    extract:
      output_format: "csv"   # CSV processing
      # json_path and filters ignored for CSV
    transform:
      input:
        format: "csv"
        header: true
    # ... rest of CSV configuration
```

## CSV Format Specifications

### Supported CSV Features

- **Quoted Fields**: Properly handles fields enclosed in double quotes
- **Embedded Commas**: Commas within quoted fields are preserved
- **Line Breaks**: Handles both `\n` and `\r\n` line endings
- **Embedded Quotes**: Double quotes within fields are supported
- **Header Rows**: Optional header row processing

### CSV Parsing Rules

1. Fields separated by commas
2. Fields may be enclosed in double quotes
3. Embedded quotes are escaped by doubling them (`""`)
4. Line breaks within quoted fields are preserved
5. Empty fields are represented as empty strings

## Error Handling

### CSV-Specific Errors

- **Parse Errors**: Invalid CSV format returns descriptive error messages
- **Column Index Errors**: Out-of-bounds `field_index` values are handled gracefully
- **Type Conversion Errors**: Failed conversions log warnings and continue processing

### Debugging

Enable debug output to troubleshoot CSV processing:

```yaml
extract:
  debug:
    enabled: true
    path: "/tmp/elasticetl-debug"
```

Debug files include:
- Raw CSV data received from APIs
- Parsed CSV structure
- Transformation results
- Error details

## Performance Considerations

### Memory Usage

- CSV data is stored as `[][]string` which is memory efficient
- Large CSV files are processed row-by-row to minimize memory footprint
- Consider using streaming for very large datasets

### Processing Speed

- CSV parsing is optimized for performance
- Column-based transformations are faster than JSON path operations
- Batch processing improves throughput for large datasets

## Migration Guide

### From JSON to CSV

To migrate an existing JSON pipeline to CSV:

1. Change `output_format` from "json" to "csv" in extract config
2. Remove `json_path` and `filters` from extract config
3. Update transform config:
   - Set `input.format` to "csv"
   - Replace `field` with `field_index` in conversion functions
4. Update load config:
   - Change `input` from "transformed_data" to "csv_data"

### Gradual Migration

You can run both JSON and CSV pipelines simultaneously:
- Keep existing JSON pipelines unchanged
- Add new CSV pipelines alongside them
- Gradually migrate data sources as needed

## Troubleshooting

### Common Issues

1. **CSV Parse Errors**:
   - Check for malformed CSV data
   - Verify quote escaping
   - Enable debug output to inspect raw data

2. **Column Index Errors**:
   - Verify `field_index` values are within CSV column range
   - Remember that indexing is 0-based

3. **Type Conversion Failures**:
   - Check data types in CSV columns
   - Verify conversion function parameters

### Debug Steps

1. Enable debug output in extract configuration
2. Check debug files for raw CSV data
3. Verify CSV structure matches configuration
4. Test with small datasets first

## Best Practices

1. **Always specify header configuration** when CSV has headers
2. **Use meaningful field indices** and document column mappings
3. **Enable debug output** during development and testing
4. **Validate CSV structure** before processing large datasets
5. **Monitor memory usage** with large CSV files
6. **Use appropriate data types** for conversion functions

## API Reference

### Configuration Fields

- `ExtractConfig.OutputFormat`: "json" | "csv"
- `TransformInputConfig.Format`: "json" | "csv"
- `TransformInputConfig.Header`: boolean
- `ConversionFunctionConfig.FieldIndex`: integer (0-based)

### Result Structure

```go
type Result struct {
    Timestamp time.Time              `json:"timestamp"`
    Source    string                 `json:"source"`
    Data      map[string]interface{} `json:"data,omitempty"`      // JSON data
    CSVData   [][]string             `json:"csv_data,omitempty"`  // CSV data
    Metadata  map[string]interface{} `json:"metadata"`
}
