# ElasticETL Configuration Validator

A comprehensive configuration validation tool for ElasticETL that validates both JSON and YAML configuration files.

## Features

- **Comprehensive Validation**: Validates all configuration sections including global settings, pipelines, extract/transform/load configurations
- **Multiple Output Formats**: Supports text, JSON, and YAML output formats
- **Error Collection**: Collects all validation errors in a single pass instead of stopping at the first error
- **Severity Levels**: Categorizes issues as errors, warnings, or informational messages
- **Helpful Suggestions**: Provides specific suggestions for fixing each validation issue
- **Schema Validation**: Validates configuration structure and data types
- **Strict Mode**: Optional strict validation mode for enhanced checking

## Installation

Build the tool from the ElasticETL project root:

```bash
cd ElasticETL
go build -o validateConfig ./cmd/validateConfig
```

## Usage

### Basic Usage

```bash
./validateConfig -config path/to/config.yaml
```

### Command Line Options

- `-config string`: Path to configuration file (required)
- `-format string`: Output format: text, json, yaml (default "text")
- `-verbose`: Enable verbose output (shows info messages)
- `-strict`: Enable strict validation mode
- `-help`: Show help message

### Examples

#### Validate with text output (default)
```bash
./validateConfig -config test-config.yaml
```

#### Validate with JSON output
```bash
./validateConfig -config test-config.yaml -format json
```

#### Validate with verbose output
```bash
./validateConfig -config test-config.yaml -verbose
```

#### Validate in strict mode
```bash
./validateConfig -config test-config.yaml -strict
```

## Output Formats

### Text Format (Default)
Human-readable format with clear sections for errors, warnings, and summary information.

### JSON Format
Machine-readable JSON format suitable for integration with other tools or CI/CD pipelines.

### YAML Format
YAML format for easy reading and processing by YAML-aware tools.

## Validation Categories

### Errors
Critical issues that prevent the configuration from working properly:
- Missing required fields
- Invalid data types or formats
- Invalid URL formats
- Invalid cron expressions
- Duplicate pipeline names

### Warnings
Issues that may cause problems but don't prevent basic functionality:
- Deprecated configuration options
- Performance concerns (very short timeouts, high retry counts)
- Security concerns (privileged ports)

### Info (Verbose Mode)
Informational messages about configuration choices:
- Multiple scheduling options specified
- Configuration recommendations

## Supported Configuration Formats

- **JSON**: `.json` files
- **YAML**: `.yaml` and `.yml` files

## Exit Codes

- `0`: Configuration is valid
- `1`: Configuration has validation errors or tool encountered an error

## Integration with CI/CD

The tool can be easily integrated into CI/CD pipelines:

```bash
# Validate configuration and fail build if invalid
./validateConfig -config production-config.yaml || exit 1

# Generate JSON report for further processing
./validateConfig -config config.yaml -format json > validation-report.json
```

## Error Codes

Each validation error includes a specific error code for programmatic handling:

- `PARSE_ERROR`: Configuration file parsing failed
- `MISSING_QUERY`: Required query field is missing
- `INVALID_URL`: URL format is invalid
- `INVALID_CRON`: Cron expression is malformed
- `DUPLICATE_PIPELINE_NAME`: Pipeline names must be unique
- `MISSING_PIPELINE_NAME`: Pipeline name is required
- And many more...

## Examples of Common Issues

### Missing Query
```
Error: [MISSING_QUERY] pipelines[0].extract.query: Query is required
Suggestion: Provide an Elasticsearch DSL query
```

### Invalid Cron Expression
```
Error: [INVALID_CRON] pipelines[0].schedule.cronSchedule: Invalid cron expression
Value: 07,17,27,37,47,57,*,*,*,*
Suggestion: Use standard cron format (e.g., '0 */5 * * * *')
```

### Deprecated Configuration
```
Warning: [DEPRECATED_INTERVAL] pipelines[0].schedule.interval: Using deprecated 'interval' field
Suggestion: Move interval to schedule.interval
```

## Contributing

When adding new validation rules:

1. Add appropriate error codes in the validation functions
2. Provide helpful error messages and suggestions
3. Categorize as error, warning, or info appropriately
4. Test with both valid and invalid configurations
