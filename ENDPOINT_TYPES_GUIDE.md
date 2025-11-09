# Endpoint Types and TLS Configuration Guide

This guide explains the new endpoint type support and TLS configuration options in ElasticETL.

## Overview

ElasticETL now supports two endpoint types:
- **Generic** (default): Standard JSON-based APIs like Elasticsearch
- **URL-encoded**: Form-based APIs like Splunk

Additionally, insecure TLS connections are supported for both endpoint types.

## Configuration Parameters

### endpoint_type

Specifies the type of endpoint to connect to:

- `"generic"` (default): Uses JSON content-type and sends query as JSON body
- `"urlencoded"`: Uses form-encoded content-type and sends query as form data

```yaml
extract:
  endpoint_type: "urlencoded"  # or "generic"
```

### insecure_tls

Controls TLS certificate verification:

- `false` (default): Secure TLS with certificate verification
- `true`: Insecure TLS that skips certificate verification

```yaml
extract:
  insecure_tls: true
```

## Endpoint Types

### Generic Endpoints (Default)

Used for Elasticsearch and similar JSON-based APIs.

**Characteristics:**
- Content-Type: `application/json`
- Query sent as JSON in request body
- Default behavior when `endpoint_type` is not specified

**Example Configuration:**
```yaml
extract:
  query: |
    {
      "query": {
        "range": {
          "@timestamp": {
            "gte": "now-1h",
            "lte": "now"
          }
        }
      }
    }
  endpoint_type: "generic"  # Optional - this is the default
  urls:
    - "https://elasticsearch:9200/logs-*/_search"
```

### URL-encoded Endpoints

Used for Splunk and similar form-based APIs.

**Characteristics:**
- Content-Type: `application/x-www-form-urlencoded`
- Query sent as form data with parameter name `search`
- Follows Splunk API conventions

**Example Configuration:**
```yaml
extract:
  query: 'search index=myindex earliest=-1h@h latest=now | table _time, host, message'
  endpoint_type: "urlencoded"
  urls:
    - "https://splunk:8089/servicesNS/admin/search/jobs/export?output_mode=csv"
```

## TLS Configuration

### Secure TLS (Default)

Default behavior with full certificate verification:

```yaml
extract:
  # insecure_tls: false  # Default - can be omitted
  urls:
    - "https://secure-endpoint.com/api"
```

### Insecure TLS

For development environments or self-signed certificates:

```yaml
extract:
  insecure_tls: true
  urls:
    - "https://self-signed-endpoint.com/api"
```

**⚠️ Security Warning:** Only use `insecure_tls: true` in development environments or when you trust the endpoint. This disables certificate verification and makes connections vulnerable to man-in-the-middle attacks.

## Complete Examples

### Splunk Configuration

```yaml
pipelines:
  - name: "splunk-pipeline"
    extract:
      query: 'search index=web earliest=-1h@h | table _time, clientip, status'
      endpoint_type: "urlencoded"
      insecure_tls: true
      urls:
        - "https://splunk.company.com:8089/servicesNS/admin/search/jobs/export?output_mode=csv"
      cluster_names:
        - "splunk-prod"
      auth_basic:
        user: "admin"
        password: "${SPLUNK_PASSWORD}"
        password_type: "ENV_VAR"
      output_format: "csv"
```

### Elasticsearch Configuration

```yaml
pipelines:
  - name: "elasticsearch-pipeline"
    extract:
      query: |
        {
          "query": {
            "bool": {
              "must": [
                {"range": {"@timestamp": {"gte": "now-1h"}}}
              ]
            }
          }
        }
      endpoint_type: "generic"  # Optional - default
      insecure_tls: false       # Optional - default
      urls:
        - "https://elasticsearch.company.com:9200/logs-*/_search"
      cluster_names:
        - "es-prod"
      auth_headers:
        - "Bearer ${ES_TOKEN}"
```

## Authentication Support

Both endpoint types support all authentication methods:

- **auth_headers**: Custom authorization headers
- **auth_basic**: Basic authentication with multiple password types
- **additional_headers**: Custom headers

## Migration from elasticsearch_query

The configuration field has been renamed:

**Old:**
```yaml
extract:
  elasticsearch_query: "SELECT * FROM logs"
```

**New:**
```yaml
extract:
  query: "SELECT * FROM logs"
```

## Troubleshooting

### Common Issues

1. **Certificate Errors**
   - Solution: Set `insecure_tls: true` for self-signed certificates
   - Better Solution: Add proper CA certificates to your system

2. **Content-Type Mismatch**
   - Ensure `endpoint_type` matches your API requirements
   - Splunk APIs typically need `"urlencoded"`
   - Elasticsearch APIs need `"generic"`

3. **Authentication Failures**
   - Verify credentials and authentication method
   - Check if additional headers are required

### Debug Mode

Enable debug output to troubleshoot issues:

```yaml
extract:
  debug:
    enabled: true
    path: "/tmp/debug"
```

This will create debug files showing the exact requests and responses.
