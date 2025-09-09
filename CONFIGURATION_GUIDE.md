# ElasticETL Configuration Guide

This guide provides an overview of all available configuration files and their use cases.

## Available Configurations

### Basic Configurations

#### 1. `configs/basic-config.yaml`
**Purpose**: Simple, minimal configuration for getting started with ElasticETL.
**Features**:
- Single pipeline with basic Elasticsearch query
- Prometheus output with debug stream
- Minimal resource limits
- JSON output format

**Use Case**: Development, testing, and learning ElasticETL basics.

#### 2. `examples/simple-example.yaml`
**Purpose**: Absolute minimal configuration for first-time users.
**Features**:
- Single aggregation query
- Debug output only
- Very low resource limits
- Perfect for testing connectivity

**Use Case**: Initial setup verification and basic functionality testing.

### Production Configurations

#### 3. `configs/production-config.yaml`
**Purpose**: Production-ready configuration with enterprise features.
**Features**:
- Multiple pipelines with different intervals
- Authentication with environment variables
- Multiple Elasticsearch endpoints with failover
- Multiple output streams (Prometheus, OTEL, CSV)
- Comprehensive error handling and retry logic
- Resource monitoring and logging

**Use Case**: Production deployments with high availability requirements.

#### 4. `configs/auth-example-config.yaml`
**Purpose**: Demonstrates authentication and security features.
**Features**:
- Environment variable substitution
- Basic authentication
- Bearer token authentication
- Custom headers
- TLS configuration

**Use Case**: Secure environments requiring authentication.

### Specialized Configurations

#### 5. `configs/debug-formats-config.yaml`
**Purpose**: Showcases debug stream formats for troubleshooting.
**Features**:
- Multiple debug formats (JSON, Prometheus, OTEL)
- Side-by-side comparison of output formats
- Production stream debugging

**Use Case**: Troubleshooting data formatting issues.

#### 6. `configs/dynamic-labels-config.yaml`
**Purpose**: Advanced label configuration for metrics.
**Features**:
- Dynamic labels from CSV data
- Static label configuration
- Complex metric naming

**Use Case**: Advanced metrics labeling and organization.

#### 7. `configs/simple-dynamic-labels.yaml`
**Purpose**: Simplified version of dynamic labels.
**Features**:
- Basic dynamic labeling
- Easy to understand examples

**Use Case**: Learning dynamic label concepts.

### CSV Processing Configurations

#### 8. `examples/csv-flattened-config.yaml`
**Purpose**: Demonstrates CSV output with flattened JSON data.
**Features**:
- JSON to CSV transformation
- Flattened data structure
- CSV-specific configuration

**Use Case**: Data export and analysis workflows.

#### 9. `examples/corrected-nested-csv-config.yaml`
**Purpose**: Handles complex nested JSON to CSV conversion.
**Features**:
- Nested array processing
- Complex data flattening
- Depth-based CSV generation

**Use Case**: Complex data structures requiring CSV export.

#### 10. `examples/nested-array-csv-config.yaml`
**Purpose**: Specialized nested array handling.
**Features**:
- Array expansion
- Nested object flattening
- Index-based field naming

**Use Case**: Processing arrays within JSON responses.

### Filter and Transformation Configurations

#### 11. `examples/regex-filters-config.yaml`
**Purpose**: Advanced filtering using regular expressions.
**Features**:
- Regex-based field filtering
- Include/exclude patterns
- Complex field matching

**Use Case**: Selective data processing and field filtering.

#### 12. `examples/debug-example.yaml`
**Purpose**: Debug-focused configuration for development.
**Features**:
- Comprehensive debug output
- Multiple debug streams
- Development-friendly settings

**Use Case**: Development and debugging workflows.

## Configuration Selection Guide

### For New Users
1. Start with `examples/simple-example.yaml`
2. Move to `configs/basic-config.yaml` once comfortable
3. Explore specialized configurations based on needs

### For Development
1. Use `configs/basic-config.yaml` for general development
2. Use `configs/debug-formats-config.yaml` for troubleshooting
3. Use `examples/debug-example.yaml` for detailed debugging

### For Production
1. Use `configs/production-config.yaml` as a template
2. Customize based on your infrastructure
3. Add authentication using `configs/auth-example-config.yaml` patterns

### For Specific Use Cases
- **CSV Export**: Use `examples/csv-flattened-config.yaml`
- **Complex Data**: Use `examples/nested-array-csv-config.yaml`
- **Filtering**: Use `examples/regex-filters-config.yaml`
- **Dynamic Labels**: Use `configs/dynamic-labels-config.yaml`

## Common Configuration Patterns

### Environment Variables
All configurations support environment variable substitution using `${VAR_NAME}` syntax:
```yaml
auth_headers:
  - "Bearer ${ES_TOKEN}"
basic_auth:
  username: "${PROMETHEUS_USER}"
  password: "${PROMETHEUS_PASS}"
```

### Multiple Endpoints
Configure multiple Elasticsearch endpoints for high availability:
```yaml
urls:
  - "https://es-primary.company.com:9200/index/_search"
  - "https://es-backup.company.com:9200/index/_search"
cluster_names:
  - "primary"
  - "backup"
```

### Stream Types
Supported stream types:
- `prometheus`: Prometheus pushgateway or remote write
- `otel`: OpenTelemetry collector
- `gem`: GEM with Prometheus remote write
- `csv`: CSV file output
- `debug`: Debug file output (JSON, Prometheus, or OTEL format)

### Output Formats
Transform output formats:
- `json`: Standard JSON format (default)
- `csv`: CSV format with flattened data

## Best Practices

1. **Start Simple**: Begin with basic configurations and add complexity gradually
2. **Use Environment Variables**: Never hardcode credentials in configuration files
3. **Enable Debug Streams**: Use debug streams during development and troubleshooting
4. **Monitor Resources**: Set appropriate resource limits for your environment
5. **Test Configurations**: Validate configurations in development before production
6. **Document Changes**: Keep track of configuration modifications
7. **Backup Configurations**: Version control your configuration files

## Configuration Validation

Before deploying configurations:
1. Validate YAML syntax
2. Test Elasticsearch connectivity
3. Verify authentication credentials
4. Check output stream endpoints
5. Monitor resource usage
6. Review log outputs

## Support and Troubleshooting

- Use debug streams to inspect data flow
- Check application logs for errors
- Verify network connectivity to endpoints
- Validate authentication credentials
- Monitor resource consumption
- Review configuration syntax

For detailed information about specific features, refer to the individual configuration files and their inline comments.
