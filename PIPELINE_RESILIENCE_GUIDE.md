# Pipeline Resilience and Retry Configuration Guide

This document describes the enhancements made to ElasticETL to improve pipeline resilience and add retry functionality.

## Overview

The ElasticETL project has been enhanced with the following key features:

1. **Pipeline Isolation**: Failed pipelines no longer affect other running pipelines
2. **Retry Mechanism**: Failed pipelines can be automatically retried after a configurable interval
3. **Enhanced Status Reporting**: Detailed pipeline status including failure information
4. **Graceful Error Handling**: Panic recovery and proper error isolation

## Configuration Changes

### New Configuration Field: `retryInterval`

A new optional field `retryInterval` has been added to the pipeline configuration:

```yaml
pipelines:
  - name: "basic-metrics"
    enabled: true
    interval: "60s"
    retryInterval: "24h"  # New field - retry failed pipelines after 24 hours
    # ... rest of configuration
```

#### `retryInterval` Options:
- **Format**: Duration string (e.g., "30s", "5m", "1h", "24h")
- **Default**: If not specified or set to "0", failed pipelines will not be retried
- **Behavior**: When a pipeline fails, it will be retried after the specified interval

## Key Features

### 1. Pipeline Isolation

**Before**: If one pipeline failed, it could potentially affect other pipelines or cause the entire application to fail.

**After**: Each pipeline runs in complete isolation:
- Panics in one pipeline are recovered and don't crash the application
- Failures in one pipeline don't stop other pipelines from running
- Each pipeline maintains its own failure state independently

### 2. Retry Mechanism

**How it works**:
1. When a pipeline fails (extraction, transformation, or loading), it's marked as failed
2. The failure time is recorded
3. The pipeline continues to run on its normal interval, but execution is skipped
4. When the `retryInterval` has passed since the last failure, the pipeline will attempt to execute again
5. If the retry succeeds, the failure state is cleared
6. If the retry fails, the failure time is updated and the cycle continues

### 3. Enhanced Status Reporting

The application now provides detailed status information including:
- Pipeline running state
- Failure status
- Last failure time
- Retry interval configuration
- Regular execution interval

**Example output**:
```
Pipeline Status:
  basic-metrics: running (FAILED) - last failure: 2023-10-27 13:15:30 - retry: 24h (interval: 60s)
  secondary-pipeline: running (interval: 5m)
```

### 4. Graceful Error Handling

All pipeline operations are wrapped with:
- **Panic recovery**: Prevents crashes from propagating
- **Proper error logging**: All failures are logged with context
- **Metrics recording**: Failures and successes are properly tracked
- **State management**: Pipeline states are consistently maintained

## Implementation Details

### Pipeline States

Each pipeline can be in one of these states:
- **Running + Successful**: Pipeline is executing normally
- **Running + Failed**: Pipeline is running but skipping execution due to recent failure
- **Stopped**: Pipeline is not running

### Failure Handling Flow

```
Pipeline Execution
       ↓
   Try Extract
       ↓
   [Success] → Try Transform
       ↓
   [Success] → Try Load
       ↓
   [Success] → Mark as Successful
       ↓
   Record Success Metrics

[Any Failure] → Mark as Failed
       ↓
   Record Failure Time
       ↓
   Log Error & Record Metrics
       ↓
   Skip Future Executions Until Retry Interval
```

### Retry Logic

```
Normal Execution Interval Tick
       ↓
   Check Pipeline State
       ↓
   [Not Failed] → Execute Normally
       ↓
   [Failed] → Check Retry Interval
       ↓
   [Retry Time Not Reached] → Skip Execution
       ↓
   [Retry Time Reached] → Execute (Retry Attempt)
```

## Configuration Examples

### Basic Configuration with Retry
```yaml
pipelines:
  - name: "metrics-pipeline"
    enabled: true
    interval: "60s"
    retryInterval: "1h"  # Retry after 1 hour
    # ... extract, transform, load config
```

### Multiple Pipelines with Different Retry Strategies
```yaml
pipelines:
  - name: "critical-metrics"
    enabled: true
    interval: "30s"
    retryInterval: "5m"   # Quick retry for critical data
    # ... config
    
  - name: "batch-processing"
    enabled: true
    interval: "1h"
    retryInterval: "24h"  # Daily retry for batch jobs
    # ... config
    
  - name: "experimental-pipeline"
    enabled: true
    interval: "5m"
    # No retryInterval - failures are permanent until manual intervention
    # ... config
```

## Monitoring and Observability

### Log Messages

The application now provides enhanced logging:

```
INFO: Pipeline Status:
INFO:   critical-metrics: running (interval: 30s)
INFO:   batch-processing: running (FAILED) - last failure: 2023-10-27 13:15:30 - retry: 24h (interval: 1h)
WARN: Pipeline batch-processing failed: extraction failed: connection timeout
INFO: Pipeline critical-metrics executed successfully (processed 150 entries in 2.3s)
```

### Metrics

All pipeline failures and successes are recorded in metrics, allowing for:
- Monitoring pipeline health
- Alerting on repeated failures
- Tracking retry success rates
- Performance analysis

## Best Practices

### 1. Retry Interval Selection
- **Short intervals (5m-1h)**: For critical, frequently-needed data
- **Medium intervals (1h-6h)**: For important but not time-critical data
- **Long intervals (12h-24h)**: For batch processing or less critical data
- **No retry**: For experimental or optional pipelines

### 2. Pipeline Design
- Design pipelines to be idempotent (safe to retry)
- Use appropriate timeouts to prevent hanging
- Implement proper error handling in custom components
- Monitor pipeline metrics and logs

### 3. Resource Management
- Consider the impact of retry attempts on system resources
- Stagger retry intervals for multiple pipelines to avoid resource spikes
- Monitor memory and CPU usage during retry attempts

## Troubleshooting

### Common Issues

1. **Pipeline stuck in failed state**
   - Check logs for the root cause of failures
   - Verify external dependencies (Elasticsearch, endpoints)
   - Consider adjusting retry intervals

2. **High resource usage during retries**
   - Stagger retry intervals across pipelines
   - Implement backoff strategies if needed
   - Monitor system resources

3. **Retry not working**
   - Verify `retryInterval` is properly configured
   - Check that the pipeline is still running (not stopped)
   - Review logs for retry attempts

### Debugging Commands

```bash
# Check pipeline status
./elasticetl --config configs/basic-config.yaml

# Monitor logs for retry attempts
tail -f /var/log/elasticetl.log | grep -E "(FAILED|retry|Pipeline.*executed)"

# Check metrics endpoint
curl http://localhost:8080/metrics | grep pipeline
```

## Migration Guide

### Existing Configurations

Existing configurations will continue to work without changes. The `retryInterval` field is optional.

### Adding Retry to Existing Pipelines

Simply add the `retryInterval` field to your pipeline configurations:

```yaml
# Before
pipelines:
  - name: "existing-pipeline"
    enabled: true
    interval: "60s"
    # ... rest of config

# After
pipelines:
  - name: "existing-pipeline"
    enabled: true
    interval: "60s"
    retryInterval: "1h"  # Add this line
    # ... rest of config
```

## Conclusion

These enhancements significantly improve the reliability and maintainability of ElasticETL pipelines by:

- Preventing cascade failures
- Automatically recovering from transient issues
- Providing better visibility into pipeline health
- Maintaining system stability even when individual pipelines fail

The retry mechanism is particularly valuable for handling temporary network issues, service outages, or resource constraints that might cause intermittent failures.
