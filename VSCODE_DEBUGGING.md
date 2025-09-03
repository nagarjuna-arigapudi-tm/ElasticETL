# VS Code Debugging Guide for ElasticETL

This guide explains how to debug ElasticETL using Visual Studio Code with the Go extension.

## Prerequisites

1. **VS Code Extensions Required:**
   - Go extension (by Google)
   - Optional: Go Test Explorer, Go Outliner

2. **Go Tools:**
   - Delve debugger (installed automatically with Go extension)
   - Go language server (gopls)

3. **Install Extensions:**
   ```bash
   # Install VS Code extensions via command line (optional)
   code --install-extension golang.go
   ```

## Quick Start

1. **Open ElasticETL project in VS Code:**
   ```bash
   cd ElasticETL
   code .
   ```

2. **Build the project:**
   - Press `Ctrl+Shift+P` (or `Cmd+Shift+P` on macOS)
   - Type "Tasks: Run Task"
   - Select "Build ElasticETL"

3. **Start debugging:**
   - Press `F5` or go to Run and Debug panel (`Ctrl+Shift+D`)
   - Select "Debug ElasticETL" configuration
   - Click the green play button

## Debug Configurations

The project includes several pre-configured debug configurations in `.vscode/launch.json`:

### 1. Debug ElasticETL (Default)
- **Purpose:** Debug with default configuration
- **Config:** `configs/config.yaml`
- **Log Level:** Debug
- **Usage:** General debugging

### 2. Debug ElasticETL with Custom Config
- **Purpose:** Debug with example configuration that has debugging enabled
- **Config:** `examples/debug-example.yaml`
- **Log Level:** Debug
- **Usage:** Testing debug features

### 3. Debug Single Pipeline
- **Purpose:** Debug a specific pipeline only
- **Config:** `configs/config.yaml`
- **Pipeline:** `elasticsearch-metrics`
- **Usage:** Isolate issues in specific pipelines

### 4. Debug Extract Component
- **Purpose:** Deep debugging of extraction logic
- **Config:** `examples/debug-example.yaml`
- **Mode:** Verbose tracing
- **Usage:** Debug extraction issues

### 5. Test Configuration Only
- **Purpose:** Validate configuration without running
- **Args:** `-validate-only`
- **Usage:** Configuration troubleshooting

### 6. Debug with Environment Variables
- **Purpose:** Debug with custom environment variables
- **Environment:** Elasticsearch URL, Prometheus URL, etc.
- **Usage:** Testing different environments

## VS Code Tasks

Use the Command Palette (`Ctrl+Shift+P`) and type "Tasks: Run Task" to access these tasks:

### Build Tasks
- **Build ElasticETL:** Compile the binary
- **Build and Run Debug:** Build and run with debug logging
- **Full Debug Setup:** Build + setup directories + clean files

### Debug Tasks
- **Setup Debug Directories:** Create `/tmp/elasticetl/debug/` directories
- **Clean Debug Files:** Remove all debug files
- **View Extract Debug Files:** List extract debug files
- **View Load Debug Files:** List load debug files
- **Show Latest Extract Debug:** Display latest extract debug JSON
- **Show Latest Load Debug:** Display latest load debug JSON

### Test Tasks
- **Run Tests:** Execute Go tests
- **Validate Configuration:** Check config file syntax
- **Check Elasticsearch Connection:** Test ES connectivity
- **Check Metrics Endpoint:** Test metrics server

## Step-by-Step Debugging

### 1. Basic Debugging Session

1. **Set breakpoints:**
   - Click in the gutter next to line numbers
   - Or press `F9` on the line you want to break

2. **Start debugging:**
   - Press `F5`
   - Select "Debug ElasticETL" configuration

3. **Debug controls:**
   - `F5`: Continue
   - `F10`: Step Over
   - `F11`: Step Into
   - `Shift+F11`: Step Out
   - `Ctrl+Shift+F5`: Restart
   - `Shift+F5`: Stop

### 2. Debugging Extract Issues

1. **Set breakpoints in extract code:**
   ```go
   // In pkg/extract/extractor.go
   func (e *Extractor) Extract(ctx context.Context) ([]*Result, error) {
       // Set breakpoint here
       var results []*Result
       // ...
   }
   ```

2. **Use "Debug Extract Component" configuration**

3. **Inspect variables:**
   - Hover over variables to see values
   - Use Variables panel in Debug sidebar
   - Add expressions to Watch panel

### 3. Debugging Transform Issues

1. **Set breakpoints in transform code:**
   ```go
   // In pkg/transform/transformer.go
   func (t *Transformer) Transform(results []*extract.Result) ([]*TransformedResult, error) {
       // Set breakpoint here
       var transformedResults []*TransformedResult
       // ...
   }
   ```

2. **Inspect transformation data:**
   - Check input `results` array
   - Verify transformation functions
   - Monitor `transformedResults` output

### 4. Debugging Load Issues

1. **Set breakpoints in load code:**
   ```go
   // In pkg/load/loader.go
   func (l *Loader) Load(ctx context.Context, results []*transform.TransformedResult) error {
       // Set breakpoint here
       // ...
   }
   ```

2. **Debug specific streams:**
   ```go
   // In stream implementations
   func (g *GEMStream) Load(ctx context.Context, results []*transform.TransformedResult) error {
       // Set breakpoint here
       samples := g.convertToPrometheusSamples(results)
       // ...
   }
   ```

## Advanced Debugging Techniques

### 1. Conditional Breakpoints

1. **Right-click on breakpoint**
2. **Select "Edit Breakpoint"**
3. **Add condition:**
   ```go
   len(results) > 0
   // or
   result.Source == "http://localhost:9200/logs-*"
   // or
   clusterName == "production"
   ```

### 2. Logpoints

1. **Right-click in gutter**
2. **Select "Add Logpoint"**
3. **Add message:**
   ```
   Processing {len(results)} results from {result.Source}
   ```

### 3. Debug Console

Use the Debug Console to:
- Evaluate expressions: `len(results)`
- Call functions: `fmt.Printf("%+v", result)`
- Inspect complex objects: `result.Metadata`

### 4. Multi-Goroutine Debugging

1. **View all goroutines in Call Stack panel**
2. **Switch between goroutines**
3. **Set breakpoints in concurrent code:**
   ```go
   // In pipeline execution
   go func(index int) {
       defer wg.Done()
       // Set breakpoint here
       result, err := e.extractFromEndpoint(ctx, index)
   }(i)
   ```

## Debugging Workflows

### Workflow 1: Configuration Issues

1. **Run "Validate Configuration" task**
2. **If validation fails:**
   - Check syntax errors in Problems panel
   - Set breakpoint in config loading code
   - Debug with "Test Configuration Only"

### Workflow 2: No Data Extracted

1. **Enable extract debug in config:**
   ```yaml
   extract:
     debug:
       enabled: true
       path: /tmp/elasticetl/debug/extract
   ```

2. **Set breakpoints in:**
   - `extractFromEndpoint` function
   - `extractDataFromResponse` function

3. **Debug and check:**
   - Elasticsearch query substitution
   - HTTP response status
   - JSON path extraction

4. **Use "Show Latest Extract Debug" task**

### Workflow 3: Transformation Problems

1. **Add debug load stream:**
   ```yaml
   load:
     streams:
       - type: debug
         config:
           path: /tmp/elasticetl/debug/load
   ```

2. **Set breakpoints in:**
   - `Transform` function
   - `transformSingle` function
   - Conversion functions

3. **Compare extract vs load debug files**

### Workflow 4: Load Stream Failures

1. **Set breakpoints in stream implementations:**
   - `GEMStream.Load`
   - `PrometheusStream.Load`
   - `OTELStream.Load`

2. **Debug HTTP requests:**
   - Check endpoint URLs
   - Inspect request headers
   - Monitor response status codes

3. **Use "Check Metrics Endpoint" task**

## Debug Output Analysis in VS Code

### 1. JSON Debug Files

1. **Install JSON Tools extension (optional)**
2. **Open debug files:**
   - Use "Show Latest Extract Debug" task
   - Or manually open files in `/tmp/elasticetl/debug/`

3. **Format JSON:**
   - `Ctrl+Shift+P` → "Format Document"
   - Or use `jq` in integrated terminal

### 2. Integrated Terminal

Use VS Code's integrated terminal for:
```bash
# View debug files
ls -la /tmp/elasticetl/debug/extract/
cat /tmp/elasticetl/debug/extract/*.json | jq .

# Test Elasticsearch directly
curl -X POST "http://localhost:9200/logs-*/_search" \
  -H "Content-Type: application/json" \
  -d '{"query":{"match_all":{}},"size":1}'

# Monitor metrics
curl -s http://localhost:8090/metrics | grep pipeline
```

## Troubleshooting VS Code Debugging

### Common Issues

1. **"Failed to launch" error:**
   - Ensure Go extension is installed
   - Check that `go` is in PATH
   - Verify project builds successfully

2. **Breakpoints not hit:**
   - Ensure code is compiled with debug symbols
   - Check that breakpoint is on executable line
   - Verify correct debug configuration is selected

3. **"could not launch process" error:**
   - Check file permissions
   - Ensure binary exists (run build task first)
   - Verify working directory is correct

4. **Variables show "optimized out":**
   - Build with debug flags: `go build -gcflags="all=-N -l"`
   - Or use `go run` instead of pre-built binary

### Debug Settings

Add to VS Code settings (`.vscode/settings.json`):
```json
{
    "go.delveConfig": {
        "dlvLoadConfig": {
            "followPointers": true,
            "maxVariableRecurse": 3,
            "maxStringLen": 400,
            "maxArrayValues": 64,
            "maxStructFields": -1
        },
        "apiVersion": 2,
        "showGlobalVariables": true
    },
    "go.buildOnSave": "package",
    "go.lintOnSave": "package",
    "go.testOnSave": false
}
```

## Keyboard Shortcuts

| Action | Windows/Linux | macOS |
|--------|---------------|-------|
| Start Debugging | `F5` | `F5` |
| Step Over | `F10` | `F10` |
| Step Into | `F11` | `F11` |
| Step Out | `Shift+F11` | `Shift+F11` |
| Continue | `F5` | `F5` |
| Stop | `Shift+F5` | `Shift+F5` |
| Restart | `Ctrl+Shift+F5` | `Cmd+Shift+F5` |
| Toggle Breakpoint | `F9` | `F9` |
| Run Task | `Ctrl+Shift+P` | `Cmd+Shift+P` |

## Best Practices

1. **Use meaningful breakpoints:**
   - Set breakpoints at function entry points
   - Break before error conditions
   - Use conditional breakpoints for specific scenarios

2. **Leverage debug output:**
   - Enable both extract and load debug streams
   - Use debug files to understand data flow
   - Compare debug output before and after changes

3. **Test incrementally:**
   - Debug one pipeline at a time
   - Test configuration changes in isolation
   - Use "Test Configuration Only" for syntax validation

4. **Monitor resources:**
   - Watch memory usage in debug sessions
   - Check goroutine counts
   - Monitor HTTP connections

5. **Use version control:**
   - Commit working configurations
   - Create debug branches for testing
   - Document debugging findings

This VS Code debugging setup provides comprehensive tools for troubleshooting ElasticETL issues efficiently within your development environment.
