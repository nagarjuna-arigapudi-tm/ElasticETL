# Regular Expression Filters Guide

This guide explains how to use regular expression patterns in ElasticETL filters to precisely control which fields are included or excluded from your extracted data.

## Overview

ElasticETL now supports full regular expression patterns in filter configurations, replacing the previous simple wildcard matching. This provides much more powerful and flexible field filtering capabilities.

## Filter Configuration

Filters are configured in the `extract` section of your pipeline configuration:

```yaml
extract:
  # ... other extract configuration ...
  filters:
    - type: "include"
      pattern: "regex_pattern_here"
    - type: "exclude" 
      pattern: "another_regex_pattern"
```

## Filter Types

### Include Filters
- **Type**: `include`
- **Behavior**: Only fields matching the pattern are included in the output
- **Usage**: Use when you want to select specific fields from a large dataset

### Exclude Filters
- **Type**: `exclude`
- **Behavior**: Fields matching the pattern are removed from the output
- **Usage**: Use when you want to remove unwanted fields while keeping most data

## Regular Expression Patterns

### Basic Patterns

#### Exact Match
```yaml
- type: "include"
  pattern: "^key$"  # Matches exactly "key"
```

#### Starts With
```yaml
- type: "include"
  pattern: "^response_time"  # Matches fields starting with "response_time"
```

#### Ends With
```yaml
- type: "include"
  pattern: "time$"  # Matches fields ending with "time"
```

#### Contains
```yaml
- type: "include"
  pattern: ".*cpu.*"  # Matches fields containing "cpu"
```

### Advanced Patterns

#### Multiple Options
```yaml
- type: "include"
  pattern: ".*(cpu|memory|disk).*"  # Matches fields containing cpu, memory, or disk
```

#### Specific Field Structure
```yaml
- type: "include"
  pattern: ".*_metrics\\.(avg|max|min|count|sum)"  # Matches metric aggregation fields
```

#### Nested Field Patterns
```yaml
- type: "include"
  pattern: "hosts\\[\\d+\\]\\.(key|cpu_usage)"  # Matches specific nested array fields
```

#### Exclude Internal Fields
```yaml
- type: "exclude"
  pattern: ".*\\.(doc_count_error_upper_bound|sum_other_doc_count).*"
```

## Common Use Cases

### 1. Performance Metrics Only

Extract only performance-related metrics:

```yaml
filters:
  - type: "include"
    pattern: ".*(response_time|cpu|memory|disk).*"
  - type: "include"
    pattern: "^key$"  # Include service/host identifiers
  - type: "exclude"
    pattern: ".*bucket.*"  # Exclude bucket metadata
```

### 2. Statistical Aggregations

Include only statistical values from Elasticsearch aggregations:

```yaml
filters:
  - type: "include"
    pattern: ".*\\.(avg|max|min|sum|count)$"
  - type: "include"
    pattern: "^key$"
  - type: "exclude"
    pattern: ".*\\.(doc_count_error_upper_bound|sum_other_doc_count).*"
```

### 3. Specific Service Metrics

Filter for specific types of metrics across services:

```yaml
filters:
  - type: "include"
    pattern: "^key$"
  - type: "include"
    pattern: ".*response_time.*"
  - type: "include"
    pattern: ".*error_rate.*"
  - type: "include"
    pattern: ".*throughput.*"
```

### 4. Host-Level Infrastructure Metrics

Extract infrastructure metrics from nested host data:

```yaml
filters:
  - type: "include"
    pattern: "^key$"  # Service name
  - type: "include"
    pattern: "hosts\\[\\d+\\]\\.key"  # Host names
  - type: "include"
    pattern: "hosts\\[\\d+\\]\\.(cpu_usage|memory_usage|disk_usage)"
  - type: "exclude"
    pattern: ".*bucket.*"
```

## Pattern Examples

### Elasticsearch Aggregation Fields

Common patterns for Elasticsearch aggregation results:

```yaml
# Include all metric aggregation values
- type: "include"
  pattern: ".*\\.(value|avg|max|min|sum|count)$"

# Include percentile values
- type: "include"
  pattern: ".*\\.values\\.(\\d+\\.\\d+|\\d+)$"

# Include bucket keys and doc counts
- type: "include"
  pattern: ".*(key|doc_count)$"

# Exclude aggregation metadata
- type: "exclude"
  pattern: ".*\\.(doc_count_error_upper_bound|sum_other_doc_count).*"
```

### Time Series Data

Patterns for time-based metrics:

```yaml
# Include timestamp fields
- type: "include"
  pattern: ".*(timestamp|time|date).*"

# Include rate metrics
- type: "include"
  pattern: ".*_rate$"

# Include counter metrics
- type: "include"
  pattern: ".*_(total|count)$"
```

### Application Metrics

Patterns for application performance monitoring:

```yaml
# HTTP metrics
- type: "include"
  pattern: ".*(http|request|response).*"

# Database metrics
- type: "include"
  pattern: ".*(db|database|query|connection).*"

# Cache metrics
- type: "include"
  pattern: ".*(cache|redis|memcached).*"
```

## Error Handling

### Invalid Regular Expressions

If a regular expression pattern is invalid, the filter will fall back to exact string matching:

```yaml
# Invalid regex - will match exactly "invalid[pattern"
- type: "include"
  pattern: "invalid[pattern"
```

### Pattern Debugging

To debug your patterns:

1. Enable debug output in your configuration
2. Check the debug files to see which fields are being matched
3. Test patterns using online regex tools
4. Start with simple patterns and gradually add complexity

## Best Practices

### 1. Order Matters

Filters are applied in the order they appear in the configuration:

```yaml
filters:
  # Apply includes first to select desired fields
  - type: "include"
    pattern: ".*metrics.*"
  
  # Then apply excludes to remove unwanted subsets
  - type: "exclude"
    pattern: ".*\\.bucket.*"
```

### 2. Escape Special Characters

Remember to escape regex special characters in YAML:

```yaml
# Correct - escaped backslashes and dots
- type: "include"
  pattern: "hosts\\[\\d+\\]\\.cpu_usage"

# Incorrect - unescaped characters
- type: "include"
  pattern: "hosts[\d+].cpu_usage"
```

### 3. Test Incrementally

Start with broad patterns and refine:

```yaml
# Step 1: Include all metrics
- type: "include"
  pattern: ".*metrics.*"

# Step 2: Refine to specific metric types
- type: "include"
  pattern: ".*_metrics\\.(avg|max|min)"

# Step 3: Add exclusions for unwanted fields
- type: "exclude"
  pattern: ".*\\.bucket.*"
```

### 4. Performance Considerations

- Simple patterns are faster than complex ones
- Include filters are generally more efficient than exclude filters
- Consider the size of your data when designing patterns

## Migration from Wildcard Patterns

If you were using the old wildcard patterns, here's how to convert them:

### Old Wildcard Format
```yaml
- type: "include"
  pattern: "*response_time*"
```

### New Regex Format
```yaml
- type: "include"
  pattern: ".*response_time.*"
```

### Conversion Guide
- `*` becomes `.*`
- `?` becomes `.`
- Add `^` for start of string matching
- Add `$` for end of string matching
- Escape special regex characters: `\.`, `\[`, `\]`, `\(`, `\)`, etc.

## Complete Example

Here's a comprehensive example showing various regex filter patterns:

```yaml
extract:
  elasticsearch_query: |
    {
      "aggs": {
        "services": {
          "terms": {"field": "service.keyword"},
          "aggs": {
            "response_metrics": {"stats": {"field": "response_time"}},
            "error_rate": {"avg": {"field": "error_rate"}},
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
  
  filters:
    # Include service identifiers
    - type: "include"
      pattern: "^key$"
    
    # Include response time statistics
    - type: "include"
      pattern: "response_metrics\\.(avg|max|min|count|sum)$"
    
    # Include error rates
    - type: "include"
      pattern: "error_rate$"
    
    # Include host-level metrics
    - type: "include"
      pattern: "hosts\\[\\d+\\]\\.(key|(cpu|memory)_usage)$"
    
    # Exclude Elasticsearch metadata
    - type: "exclude"
      pattern: ".*\\.(doc_count_error_upper_bound|sum_other_doc_count).*"
    
    # Exclude bucket metadata
    - type: "exclude"
      pattern: ".*bucket.*"
```

This configuration will extract only the essential performance metrics while filtering out unnecessary metadata and internal Elasticsearch fields.
