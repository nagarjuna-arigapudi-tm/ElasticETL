# ElasticETL - Comprehensive Functional Specification

## Table of Contents

1. [System Overview](#system-overview)
2. [Functional Requirements](#functional-requirements)
3. [User Stories](#user-stories)
4. [Feature Specifications](#feature-specifications)
5. [Configuration Management](#configuration-management)
6. [Data Processing Workflows](#data-processing-workflows)
7. [Debug and Monitoring](#debug-and-monitoring)
8. [Security and Authentication](#security-and-authentication)
9. [Pipeline Management](#pipeline-management)
10. [Stream Processing](#stream-processing)
11. [Error Handling and Resilience](#error-handling-and-resilience)
12. [Performance Requirements](#performance-requirements)
13. [Integration Specifications](#integration-specifications)
14. [Deployment and Operations](#deployment-and-operations)
15. [Use Cases and Examples](#use-cases-and-examples)

## System Overview

ElasticETL is a comprehensive ETL (Extract, Transform, Load) pipeline system designed to extract data from Elasticsearch and other compatible data sources, transform it according to configurable rules, and load it into various monitoring and analytics platforms. The system provides enterprise-grade features including multi-pipeline support, comprehensive debugging, authentication, resilience mechanisms, and extensive monitoring capabilities.

### Primary Use Cases

1. **Monitoring Data Pipeline**: Extract metrics from Elasticsearch and send to Prometheus/Grafana
2. **Log Analytics**: Process log data and export to various analytics platforms
3. **Data Migration**: Transform and migrate data between different systems
4. **Real-time Metrics**: Generate time-series data from structured logs
5. **Compliance Reporting**: Extract and format data for compliance requirements
6. **Performance Monitoring**: Collect and process application performance metrics

## Functional Requirements

### FR-001: Multi-Pipeline Support
**Description**: The system shall support running multiple independent ETL pipelines concurrently.

**Acceptance Criteria**:
- Support for 100+ concurrent pipelines
- Each pipeline operates independently with its own configuration
- Pipeline failures do not affect other running pipelines
- Resource isolation between pipelines
- Individual pipeline start/stop/restart capabilities

### FR-002: Data Extraction
**Description**: The system shall extract data from multiple data sources with flexible querying capabilities.

**Acceptance Criteria**:
- Support for Elasticsearch DSL queries
- Support for Splunk-style URL-encoded queries
- Multiple endpoint support with failover
- JSON path-based data extraction
- CSV data parsing capabilities
- Regex-based field filtering

### FR-003: Data Transformation
**Description**: The system shall transform extracted data according to configurable rules.

**Acceptance Criteria**:
- Type conversions (string, int, float, bool)
- Unit conversions (bytes to KB/MB/GB)
- Null value handling and substitution
- JSON to CSV conversion with flattening
- Regex-based field processing
- Stateless and stateful processing modes

### FR-004: Data Loading
**Description**: The system shall load transformed data into multiple output destinations.

**Acceptance Criteria**:
- Support for Prometheus (pushgateway/remote write)
- Support for OpenTelemetry collectors
- Support for GEM endpoints
- CSV file output
- Debug output in multiple formats
- Concurrent loading to multiple streams

### FR-005: Authentication and Security
**Description**: The system shall provide secure authentication mechanisms for all external connections.

**Acceptance Criteria**:
- Bearer token authentication
- Basic authentication with password encryption
- Custom header support
- Environment variable substitution
- TLS configuration with certificate validation
- Secure credential management

### FR-006: Pipeline Resilience
**Description**: The system shall provide robust error handling and recovery mechanisms.

**Acceptance Criteria**:
- Automatic retry for failed pipelines
- Configurable retry intervals
- Pipeline failure isolation
- Graceful error handling with panic recovery
- Circuit breaker pattern implementation
- Failure state tracking and reporting

### FR-007: Scheduling System
**Description**: The system shall support flexible scheduling mechanisms for pipeline execution.

**Acceptance Criteria**:
- Cron expression support
- Interval-based scheduling
- Immediate execution capability
- Schedule validation and error reporting
- Graceful shutdown handling
- Timezone support

### FR-008: Debug and Monitoring
**Description**: The system shall provide comprehensive debugging and monitoring capabilities.

**Acceptance Criteria**:
- Stage-specific debug output (extract, transform, load)
- Pipeline-specific debug files with timestamps
- Configurable debug granularity
- Built-in Prometheus metrics
- Health check endpoints
- Performance monitoring and reporting

### FR-009: Configuration Management
**Description**: The system shall support flexible configuration management with hot reload capabilities.

**Acceptance Criteria**:
- YAML-based configuration
- Configuration validation
- Hot reload without service restart
- Environment variable substitution
- Configuration versioning support
- Backup and rollback capabilities

### FR-010: Resource Management
**Description**: The system shall provide resource management and limits to ensure stable operation.

**Acceptance Criteria**:
- Memory usage limits
- CPU usage limits
- Goroutine management
- Connection pooling
- Resource monitoring and alerting
- Automatic resource cleanup

## User Stories

### US-001: DevOps Engineer - Basic Pipeline Setup
**As a** DevOps engineer  
**I want to** set up a basic ETL pipeline to extract metrics from Elasticsearch and send them to Prometheus  
**So that** I can monitor application performance in Grafana

**Acceptance Criteria**:
- Create a simple configuration file
- Configure Elasticsearch connection with authentication
- Set up Prometheus output stream
- Verify data flow with debug output
- Monitor pipeline health and performance

### US-002: Data Analyst - CSV Data Export
**As a** data analyst  
**I want to** extract log data from Elasticsearch and export it as CSV files  
**So that** I can perform offline analysis using spreadsheet tools

**Acceptance Criteria**:
- Configure JSON to CSV transformation
- Handle nested data structures
- Apply data filtering and type conversions
- Export to timestamped CSV files
- Validate data integrity and completeness

### US-003: Security Administrator - Encrypted Authentication
**As a** security administrator  
**I want to** configure encrypted authentication for all external connections  
**So that** credentials are not exposed in configuration files

**Acceptance Criteria**:
- Use environment variables for sensitive data
- Encrypt passwords using built-in utilities
- Configure TLS for all connections
- Validate certificate chains
- Audit authentication attempts

### US-004: Site Reliability Engineer - Pipeline Resilience
**As an** SRE  
**I want to** configure automatic retry and failure handling for pipelines  
**So that** temporary outages don't cause permanent data loss

**Acceptance Criteria**:
- Configure retry intervals for different failure types
- Set up alerting for persistent failures
- Implement circuit breaker patterns
- Monitor failure rates and recovery times
- Ensure data consistency during failures

### US-005: Developer - Debug and Troubleshooting
**As a** developer  
**I want to** enable comprehensive debugging for pipeline stages  
**So that** I can troubleshoot data processing issues effectively

**Acceptance Criteria**:
- Enable stage-specific debug output
- Generate timestamped debug files
- Inspect data at each processing stage
- Validate transformations and conversions
- Identify performance bottlenecks

### US-006: Operations Manager - Multi-Environment Deployment
**As an** operations manager  
**I want to** deploy ElasticETL across multiple environments with different configurations  
**So that** I can maintain consistent data processing across dev, staging, and production

**Acceptance Criteria**:
- Environment-specific configuration management
- Automated deployment pipelines
- Configuration validation and testing
- Resource allocation per environment
- Monitoring and alerting setup

## Feature Specifications

### Feature: Multi-Pipeline Execution

**Description**: Support for running multiple independent ETL pipelines concurrently with complete isolation.

**Functional Requirements**:
- Pipeline lifecycle management (create, start, stop, update, delete)
- Independent configuration per pipeline
- Resource isolation and management
- Concurrent execution without interference
- Individual pipeline monitoring and status reporting

**Technical Implementation**:
- Pipeline Manager coordinates all pipeline instances
- Each pipeline runs in its own goroutine
- Shared resource pool with limits and quotas
- Pipeline-specific metrics and logging
- Graceful shutdown with cleanup

**Configuration Example**:
```yaml
pipelines:
  - name: "app-metrics"
    enabled: true
    interval: "60s"
    # ... pipeline configuration
  - name: "log-processing"
    enabled: true
    interval: "300s"
    # ... pipeline configuration
```

### Feature: Advanced Data Extraction

**Description**: Flexible data extraction from multiple sources with sophisticated querying and filtering capabilities.

**Functional Requirements**:
- Multiple endpoint support with automatic failover
- Elasticsearch DSL query support
- Splunk-style URL-encoded query support
- JSON path-based data extraction
- Regex-based field filtering (include/exclude)
- CSV data parsing with header detection

**Technical Implementation**:
- HTTP client with connection pooling
- Query template processing with macro substitution
- JSON path evaluation using gjson library
- Regex compilation and caching
- CSV parser with quote handling
- Error handling with exponential backoff retry

**Configuration Example**:
```yaml
extract:
  elasticsearch_query: |
    {
      "query": {"range": {"@timestamp": {"gte": "now-5m"}}},
      "aggs": {"avg_response_time": {"avg": {"field": "response_time"}}}
    }
  urls:
    - "https://es-primary.company.com:9200/logs-*/_search"
    - "https://es-backup.company.com:9200/logs-*/_search"
  json_path: "aggregations"
  filters:
    - type: "include"
      pattern: "^avg_.*"
    - type: "exclude"
      pattern: ".*\\.keyword$"
```

### Feature: Comprehensive Data Transformation

**Description**: Advanced data transformation capabilities with type conversions, unit conversions, and format transformations.

**Functional Requirements**:
- Type conversions between string, int, float, bool
- Unit conversions (bytes to KB/MB/GB)
- Null value handling and substitution
- JSON to CSV conversion with intelligent flattening
- Regex-based field matching and processing
- Stateless and stateful processing modes

**Technical Implementation**:
- Conversion function registry with extensible architecture
- Depth-based JSON flattening algorithm
- Dynamic state tracking for CSV generation
- Previous results retention for stateful processing
- Comprehensive error handling and validation

**Configuration Example**:
```yaml
transform:
  stateless: false
  substitute_zeros_for_null: true
  output_format: "csv"
  conversion_functions:
    - field: "_source.memory_bytes"
      function: "convert_to_mb"
      from_unit: "bytes"
    - field: "_source.cpu_usage"
      function: "convert_type"
      from_type: "string"
      to_type: "float"
```

### Feature: Multi-Stream Data Loading

**Description**: Concurrent loading of transformed data to multiple output destinations with format-specific optimizations.

**Functional Requirements**:
- Prometheus pushgateway and remote write support
- OpenTelemetry collector integration
- GEM endpoint support
- CSV file output with timestamping
- Debug output in multiple formats (JSON, Prometheus, OTEL)
- CSV-based time series generation

**Technical Implementation**:
- Stream factory pattern for extensible output types
- Concurrent loading with error isolation
- Format-specific serialization and optimization
- Authentication handling per stream
- Retry logic with exponential backoff

**Configuration Example**:
```yaml
load:
  streams:
    - type: "prometheus"
      config:
        endpoint: "https://prometheus.company.com/api/v1/write"
      basic_auth:
        username: "${PROMETHEUS_USER}"
        password: "${PROMETHEUS_PASS}"
    - type: "csv"
      config:
        path: "/data/exports/metrics-{{.Date}}.csv"
    - type: "debug"
      config:
        path: "/tmp/debug"
        format: "json"
```

### Feature: Comprehensive Debug System

**Description**: Stage-specific debugging with granular control over captured information and organized output.

**Functional Requirements**:
- Extract stage debugging (query, API response, output)
- Transform stage debugging (input, transformed data, final output)
- Pipeline-specific debug files with timestamps
- Configurable debug granularity
- Multiple debug output formats
- Debug file organization and management

**Technical Implementation**:
- Debug coordinator at pipeline level
- Stage-specific debug methods
- Timestamp-based file naming
- JSON structured output
- Configurable debug paths
- Automatic directory creation

**Configuration Example**:
```yaml
extract:
  debug:
    path: "debug/extract"
    final_query: true
    api_response: true
    final_output: true

transform:
  debug:
    path: "debug/transform"
    input: true
    transformed_output: true
    final_output: true
```

### Feature: Enterprise Authentication

**Description**: Comprehensive authentication and security mechanisms for all external connections.

**Functional Requirements**:
- Bearer token authentication with environment variables
- Basic authentication with password encryption
- Custom header support
- TLS configuration with certificate validation
- Environment variable substitution throughout configuration
- Password encryption utilities

**Technical Implementation**:
- Authentication handler interface
- Environment variable substitution engine
- Password encryption/decryption utilities
- TLS configuration management
- Secure credential storage patterns

**Configuration Example**:
```yaml
extract:
  auth_basic:
    user: "${ES_USERNAME}"
    password: "${ES_PASSWORD_ENCRYPTED}"
    password_type: "encrypted"
    passkey: "${ENCRYPTION_KEY}"
  additional_headers:
    - ["X-API-Key: ${API_KEY}", "X-Environment: production"]
  insecure_tls: false
```

## Configuration Management

### Configuration Structure

The system uses a hierarchical YAML configuration structure with the following main sections:

1. **Global Configuration**: System-wide settings
2. **Pipeline Configuration**: Individual pipeline definitions
3. **Extract Configuration**: Data source and extraction settings
4. **Transform Configuration**: Data transformation rules
5. **Load Configuration**: Output destination settings

### Configuration Validation

**Requirements**:
- Schema validation on startup
- Runtime configuration validation
- Environment variable validation
- Connectivity testing
- Configuration syntax checking

**Implementation**:
- JSON Schema validation
- Custom validation rules
- Pre-flight checks
- Configuration test mode
- Validation error reporting

### Hot Reload

**Requirements**:
- Configuration file watching
- Graceful pipeline restart
- Zero-downtime updates
- Rollback capability
- Change notification

**Implementation**:
- File system watcher
- Configuration diff detection
- Pipeline update coordination
- Rollback state management
- Change event logging

## Data Processing Workflows

### Workflow 1: Elasticsearch to Prometheus

```
Elasticsearch Query
       ↓
JSON Path Extraction
       ↓
Data Flattening
       ↓
Type Conversions
       ↓
Prometheus Format
       ↓
Remote Write API
```

**Use Case**: Monitor application metrics stored in Elasticsearch logs
**Configuration**: Basic Elasticsearch extraction with Prometheus output
**Data Flow**: JSON aggregations → flattened metrics → Prometheus time series

### Workflow 2: Log Data to CSV Export

```
Elasticsearch Logs
       ↓
Field Filtering
       ↓
JSON Flattening
       ↓
CSV Generation
       ↓
File Export
```

**Use Case**: Export log data for offline analysis
**Configuration**: JSON to CSV transformation with field filtering
**Data Flow**: Structured logs → filtered fields → CSV rows → timestamped files

### Workflow 3: Multi-Source Aggregation

```
Multiple Elasticsearch Clusters
       ↓
Concurrent Extraction
       ↓
Data Aggregation
       ↓
Transformation
       ↓
Multiple Output Streams
```

**Use Case**: Aggregate metrics from multiple data centers
**Configuration**: Multiple endpoints with concurrent processing
**Data Flow**: Distributed sources → unified processing → multiple destinations

### Workflow 4: Real-time Monitoring Pipeline

```
Cron Schedule (Every 30s)
       ↓
Recent Data Extraction
       ↓
Metric Calculation
       ↓
Time Series Generation
       ↓
Prometheus + OTEL Output
```

**Use Case**: Real-time application monitoring
**Configuration**: High-frequency extraction with multiple monitoring outputs
**Data Flow**: Recent logs → calculated metrics → time series → monitoring systems

## Debug and Monitoring

### Debug Capabilities

**Extract Stage Debug**:
- Final processed query with macro substitution
- API response metadata and size information
- Extracted data before transformation
- Error details and retry attempts

**Transform Stage Debug**:
- Input data from extract stage
- Intermediate transformation results
- Final output data structure
- Conversion function results and errors

**Load Stage Debug**:
- Formatted data for each output stream
- Authentication and connection details
- Transmission results and errors
- Performance metrics per stream

### Monitoring Features

**Pipeline Metrics**:
- Execution count and duration
- Success/failure rates
- Data volume processed
- Error categorization

**System Metrics**:
- Memory and CPU usage
- Goroutine count
- Connection pool status
- Resource utilization

**Health Checks**:
- Overall system health
- Individual pipeline status
- External dependency status
- Resource availability

## Security and Authentication

### Authentication Methods

**Bearer Token Authentication**:
- Environment variable substitution
- Token rotation support
- Automatic header formatting
- Error handling and retry

**Basic Authentication**:
- Username/password combinations
- Password encryption support
- Environment variable integration
- Secure credential storage

**Custom Headers**:
- API key authentication
- Custom authentication schemes
- Multiple header support
- Dynamic header generation

### Security Features

**TLS Configuration**:
- Certificate validation
- Insecure mode for testing
- Custom CA support (planned)
- Protocol version control

**Credential Management**:
- Environment variable substitution
- Password encryption utilities
- Secure configuration patterns
- Audit trail support

## Pipeline Management

### Lifecycle Management

**Pipeline States**:
- **Stopped**: Pipeline is not running
- **Starting**: Pipeline is initializing
- **Running**: Pipeline is executing normally
- **Failed**: Pipeline has encountered errors
- **Stopping**: Pipeline is shutting down

**State Transitions**:
- Stopped → Starting → Running
- Running → Failed (on error)
- Failed → Running (on retry success)
- Any State → Stopping → Stopped

### Resource Management

**Memory Management**:
- Per-pipeline memory limits
- Garbage collection optimization
- Memory usage monitoring
- OOM protection

**CPU Management**:
- CPU usage limits
- Goroutine pool management
- Load balancing
- Performance monitoring

**Connection Management**:
- HTTP connection pooling
- Connection limits per endpoint
- Connection reuse optimization
- Timeout management

## Stream Processing

### Stream Types

**Prometheus Stream**:
- Pushgateway support
- Remote write API support
- Metric formatting and labeling
- Time series generation from CSV

**OpenTelemetry Stream**:
- OTEL collector integration
- Metric and trace support
- Resource attribute management
- Protocol buffer formatting

**GEM Stream**:
- GEM-specific formatting
- Prometheus remote write compatibility
- Custom label handling
- Authentication integration

**CSV Stream**:
- File-based output
- Timestamped filenames
- Header management
- Data validation

**Debug Stream**:
- Multiple format support (JSON, Prometheus, OTEL)
- Human-readable output
- Development and troubleshooting
- Performance analysis

### Time Series Generation

**CSV-Based Metrics**:
- Group data by unique field combinations
- Extract timestamps and values
- Generate dynamic labels
- Create separate time series per group

**Label Management**:
- Static labels from configuration
- Dynamic labels from data
- Label validation and sanitization
- Cardinality management

## Error Handling and Resilience

### Error Categories

**Transient Errors**:
- Network timeouts
- Temporary service unavailability
- Rate limiting
- Connection failures

**Permanent Errors**:
- Authentication failures
- Configuration errors
- Data format errors
- Permission issues

### Resilience Patterns

**Retry Logic**:
- Exponential backoff
- Maximum retry limits
- Retry interval configuration
- Error-specific retry policies

**Circuit Breaker**:
- Failure threshold detection
- Automatic service isolation
- Recovery testing
- Fallback mechanisms

**Graceful Degradation**:
- Partial failure handling
- Service isolation
- Fallback data sources
- Reduced functionality modes

## Performance Requirements

### Throughput Requirements

| Metric | Minimum | Target | Maximum |
|--------|---------|--------|---------|
| Pipelines | 10 | 50 | 100+ |
| Data Rate | 1MB/s | 5MB/s | 10MB/s |
| Query Frequency | 1/min | 1/30s | 1/s |
| Response Time | <10s | <5s | <1s |

### Resource Requirements

| Resource | Light Load | Normal Load | Heavy Load |
|----------|------------|-------------|------------|
| Memory | 128MB | 512MB | 1GB |
| CPU | 10% | 25% | 50% |
| Disk I/O | 1MB/s | 10MB/s | 50MB/s |
| Network | 1Mbps | 10Mbps | 100Mbps |

### Scalability Requirements

**Horizontal Scaling**:
- Multiple instance deployment
- Load distribution
- State synchronization
- Configuration sharing

**Vertical Scaling**:
- Resource limit adjustment
- Performance optimization
- Memory management
- CPU utilization

## Integration Specifications

### Elasticsearch Integration

**Supported Versions**: 7.x, 8.x
**Query Types**: DSL queries, aggregations, search queries
**Authentication**: Basic auth, API keys, bearer tokens
**Features**: Multi-cluster support, failover, connection pooling

### Prometheus Integration

**Supported Versions**: 2.x
**Protocols**: Pushgateway, Remote Write API
**Formats**: Exposition format, Protocol buffers
**Features**: Label management, time series generation, authentication

### OpenTelemetry Integration

**Supported Versions**: 1.x
**Protocols**: OTLP/gRPC, OTLP/HTTP
**Data Types**: Metrics, traces (planned)
**Features**: Resource attributes, instrumentation metadata

### Kubernetes Integration

**Deployment**: StatefulSet, Deployment
**Configuration**: ConfigMaps, Secrets
**Monitoring**: ServiceMonitor, PodMonitor
**Networking**: Services, Ingress

## Deployment and Operations

### Deployment Options

**Standalone Deployment**:
- Single binary execution
- Local configuration file
- File-based logging
- Local debug output

**Container Deployment**:
- Docker container
- Environment variable configuration
- Volume mounts for data
- Container orchestration

**Kubernetes Deployment**:
- Helm chart deployment
- ConfigMap configuration
- Secret management
- Service discovery

### Operational Procedures

**Startup Procedure**:
1. Load and validate configuration
2. Initialize system components
3. Start metrics server
4. Initialize pipelines
5. Begin pipeline execution

**Shutdown Procedure**:
1. Stop accepting new requests
2. Complete in-flight operations
3. Stop all pipelines gracefully
4. Close external connections
5. Clean up resources

**Update Procedure**:
1. Validate new configuration
2. Create backup of current state
3. Apply configuration changes
4. Restart affected pipelines
5. Verify successful update

## Use Cases and Examples

### Use Case 1: Application Performance Monitoring

**Scenario**: Monitor web application performance metrics stored in Elasticsearch and visualize them in Grafana via Prometheus.

**Configuration**:
```yaml
pipelines:
  - name: "app-performance"
    enabled: true
    interval: "30s"
    
    extract:
      elasticsearch_query: |
        {
          "query": {"range": {"@timestamp": {"gte": "now-1m"}}},
          "aggs": {
            "avg_response_time": {"avg": {"field": "response_time"}},
            "error_rate": {"terms": {"field": "status_code"}}
          }
        }
      urls: ["https://elasticsearch.company.com:9200/app-logs-*/_search"]
      json_path: "aggregations"
    
    transform:
      output_format: "csv"
      conversion_functions:
        - field: "avg_response_time.value"
          function: "convert_type"
          from_type: "string"
          to_type: "float"
    
    load:
      streams:
        - type: "prometheus"
          config:
            endpoint: "https://prometheus.company.com/api/v1/write"
            metrics:
              - name: "app_response_time"
                uniquefieldsIndex: []
                value: 0
                timestamp: 1
                labels:
                  - label_name: "application"
                    static_value: "web-app"
```

### Use Case 2: Log Data Export for Compliance

**Scenario**: Export security logs from Elasticsearch to CSV files for compliance auditing.

**Configuration**:
```yaml
pipelines:
  - name: "security-audit"
    enabled: true
    schedule:
      cron_schedule: "0 0 2 * * *"  # Daily at 2 AM
    
    extract:
      elasticsearch_query: |
        {
          "query": {
            "bool": {
              "must": [
                {"range": {"@timestamp": {"gte": "now-1d"}}},
                {"term": {"log_type": "security"}}
              ]
            }
          }
        }
      urls: ["https://elasticsearch.company.com:9200/security-logs-*/_search"]
      output_format: "csv"
    
    transform:
      input:
        format: "csv"
        header: true
      drop_null_values: true
      conversion_functions:
        - field_index: 0
          function: "convert_type"
          from_type: "string"
          to_type: "string"
    
    load:
      streams:
        - type: "csv"
          config:
            path: "/compliance/security-logs-{{.Date}}.csv"
```

### Use Case 3: Multi-Cluster Monitoring

**Scenario**: Aggregate metrics from multiple Elasticsearch clusters and send to both Prometheus and OpenTelemetry.

**Configuration**:
```yaml
pipelines:
  - name: "multi-cluster-metrics"
    enabled: true
    interval: "60s"
    retryInterval: "5m"
    
    extract:
      elasticsearch_query: |
        {
          "aggs": {
            "cluster_health": {"terms": {"field": "cluster.name"}},
            "node_count": {"cardinality": {"field": "node.id"}}
          }
        }
      urls:
        - "https://es-us-east.company.com:9200/_cluster/stats"
        - "https://es-us-west.company.com:9200/_cluster/stats"
        - "https://es-eu.company.com:9200/_cluster/stats"
      cluster_names: ["us-east", "us-west", "eu"]
      json_path: "aggregations"
      auth_headers:
        - "Bearer ${ES_US_EAST_TOKEN}"
        - "Bearer ${ES_US_WEST_TOKEN}"
        - "Bearer ${ES_EU_TOKEN}"
    
    transform:
      output_format: "csv"
      stateless: true
    
    load:
      streams:
        - type: "prometheus"
          config:
            endpoint: "https://prometheus.company.com/api/v1/write"
          basic_auth:
            username: "${PROMETHEUS_USER}"
            password: "${PROMETHEUS_PASS}"
        - type: "otel"
          config:
            endpoint: "https://otel-collector.company.com:4317"
          labels:
            service.name: "elasticsearch-monitoring"
```

### Use Case 4: Development and Debugging

**Scenario**: Set up comprehensive debugging for pipeline development and troubleshooting.

**Configuration**:
```yaml
pipelines:
  - name: "debug-pipeline"
    enabled: true
    interval: "120s"
    
    extract:
      elasticsearch_query: |
        {
          "query": {"match_all": {}},
          "size": 10
        }
      urls: ["http://localhost:9200/test-index/_search"]
      debug:
        path: "debug/extract"
        final_query: true
        api_response: true
        final_output: true
    
    transform:
      output_format: "csv"
      debug:
        path: "debug/transform"
        input: true
        transformed_output: true
        final_output: true
    
    load:
      streams:
        - type: "debug"
          config:
            path: "debug/load"
            format: "json"
        - type: "debug"
          config:
            path: "debug/prometheus"
            format: "prometheus"
        - type: "debug"
          config:
            path: "debug/otel"
            format: "otel"
```

This comprehensive functional specification provides detailed coverage of all ElasticETL features, requirements, and use cases. It serves as a complete reference for understanding the system's capabilities and how to configure it for various scenarios.
