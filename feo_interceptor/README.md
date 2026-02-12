# FEO Interceptor Module

This Caddy module provides FEO (Frontend Environment Orchestrator) interception capabilities for the frontend development proxy. It intercepts Chrome Service API responses and merges them with local Frontend CRD configurations.

## Architecture

The FEO interceptor uses a JavaScript runtime (Goja) to execute interceptor logic. The JavaScript code is **not** maintained in this repository but is instead sourced from the `@redhat-cloud-services/frontend-components-config-utilities` package.

### Thread Safety

The FEO interceptor uses a **VM-per-Request architecture** to ensure thread safety:

- **Compiled program is shared**: `goja.Program` is compiled once during provisioning (thread-safe)
- **New VM per request**: A fresh `goja.Runtime` is created for each request (prevents concurrency issues)
- **Runtime instances are independent**: Each HTTP request runs in isolation
- **No explicit locks needed**: Concurrency achieved through VM isolation
- **Cache protected by RWMutex**: CRD cache uses `sync.RWMutex` for thread-safe access

This design ensures unlimited parallel requests without threading crashes or data races.

### File Watching & Caching

To optimize performance and enable real-time updates, the interceptor:

1. **Loads CRD on startup** - Parsed and cached in memory during `Provision()`
2. **Watches for file changes** - Uses `fsnotify` to detect modifications
3. **Auto-reloads on change** - Updates cache when CRD file is modified
4. **Uses cache for requests** - Avoids per-request file I/O

**Performance benefits:**
- First request: ~10-15ms (includes cache lookup)
- Subsequent requests: ~5-8ms (cached data)
- File changes: Auto-reload in <100ms

**Cache flow:**
```
Startup → loadCRD() → Parse YAML → Store in crdCache
          ↓
          startFileWatcher() → fsnotify monitoring
                                ↓
File changed → watcherLoop() → loadCRD() → Update crdCache
                                ↓
Request → _GO_READ_FILE → Returns cached data (not file read)
```

### Why Source from frontend-components?

1. **Single Source of Truth**: The interceptor logic is maintained in one place and shared across multiple proxies
2. **Consistent Behavior**: All proxies use the same interceptor logic, ensuring consistent behavior
3. **Easier Maintenance**: Updates to interceptor logic only need to be made once in frontend-components
4. **Version Control**: Proxies can specify which version of the interceptors they want to use

## Files

- `middleware.go` - Caddy middleware implementation
- `module.go` - Caddy module registration
- `caddyfile.go` - Caddyfile directive parser
- `interceptors_bundled.js` - Bundled JavaScript from frontend-components
- `update-interceptors.sh` - Script to update the bundled interceptors
- `README.md` - This file

## Updating Interceptors

The interceptors are bundled from the TypeScript source in frontend-components and distributed as part of the npm package.

### Method 1: Using the Update Script

```bash
# Update to the latest version
./update-interceptors.sh

# Update to a specific version
./update-interceptors.sh 4.7.14
```

### Method 2: Manual Update

1. Download the package:
   ```bash
   npm pack @redhat-cloud-services/frontend-components-config-utilities
   ```

2. Extract the tarball and copy the bundle:
   ```bash
   tar -xzf redhat-cloud-services-frontend-components-config-utilities-*.tgz
   cp package/standalone/feo-interceptors.js interceptors_bundled.js
   ```

### Method 3: Build from Source (Development)

If you're developing changes to the interceptors in frontend-components:

1. Navigate to frontend-components:
   ```bash
   cd /path/to/frontend-components/packages/config-utils
   ```

2. Build the interceptors:
   ```bash
   npm run build:interceptors
   ```

3. Copy the bundle:
   ```bash
   cp standalone/feo-interceptors.js /path/to/frontend-development-proxy/feo_interceptor/interceptors_bundled.js
   ```

## How It Works

1. **Compile Phase**: The interceptor JavaScript is compiled once during Caddy provisioning for better performance
2. **Request Phase**: For each intercepted request, a new Goja VM is created (for thread safety)
3. **Bridge Functions**: Go functions are registered in the VM to allow JavaScript to read files, parse YAML, and log
4. **Processing**: The JavaScript `processRequest` function is called with the request data and bridge functions
5. **Response**: The processed JSON is returned to the client

## Intercepted Endpoints

The module intercepts these Chrome Service API endpoints:

- `/api/chrome-service/v1/static/bundles-generated.json` (navigation)
- `/api/chrome-service/v1/static/fed-modules-generated.json` (module federation)
- `/api/chrome-service/v1/static/search-index-generated.json` (search)
- `/api/chrome-service/v1/static/service-tiles-generated.json` (service tiles)
- `/api/chrome-service/v1/static/widget-registry-generated.json` (widgets)

## Configuration

In your `Caddyfile`:

```caddyfile
feo_interceptor {
    crd_path /config/deploy/frontend.yaml
}
```

## Development

When making changes to the interceptor logic:

1. Make changes in `frontend-components/packages/config-utils/src/feo/`
2. Build the bundle: `npm run build:interceptors`
3. Test the bundle in this proxy
4. Once stable, publish a new version of frontend-components-config-utilities
5. Update both proxies to use the new version

## Why Bundled from Frontend Components?

The bundled approach provides:

- **Single source of truth** - One canonical implementation
- **Type safety** - TypeScript development in frontend-components
- **Automated testing** - Comprehensive unit tests
- **Consistent behavior** - All proxies use identical logic
- **Easy updates** - Version-controlled via npm

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `ENABLE_FEO` | `true` | Enable/disable FEO interceptor (`true`, `false`, `1`, `0`) |
| `FEO_CRD_PATH` | `config/deploy/frontend.yaml` | Path to Frontend CRD file (absolute or relative) |

**Examples:**

```bash
# Disable FEO interceptor
ENABLE_FEO=false ./frontend-development-proxy

# Use custom CRD path
FEO_CRD_PATH=/etc/frontend/custom.yaml ./frontend-development-proxy

# Container with custom configuration
podman run -e ENABLE_FEO=true -e FEO_CRD_PATH=/custom/path.yaml ...
```

### Caddyfile Configuration

```caddyfile
{
    order feo_interceptor before reverse_proxy  # REQUIRED: Define module order
}

http://example.com {
    route {
        feo_interceptor {
            crd_path config/deploy/frontend.yaml  # Path to CRD file
        }
        reverse_proxy upstream:8080
    }
}
```

**Important:** The `order` directive in the global section is **required** for the middleware to load correctly.

## Testing

### Unit Tests

Run unit tests to verify core functionality:

```bash
cd feo_interceptor
go test -v
```

**Test coverage:**
- `TestShouldIntercept` - URL pattern matching for Chrome Service endpoints
- `TestRegisterBridgeFunctions` - Bridge function registration in Goja VM
- `TestFileWatcher` - File watching and cache update mechanism
- `TestProvisionWithEnvVar` - Environment variable handling
- `TestFileExists` - Helper function verification

### Integration Tests

Run end-to-end integration test:

```bash
./test-feo-integration.sh
```

This test:
1. Builds the proxy binary
2. Starts a mock upstream server
3. Starts the proxy with FEO enabled
4. Verifies data merging in responses
5. Cleans up resources

### Manual Testing

See `TESTING_GUIDE.md` for detailed manual testing procedures.

## Troubleshooting

### Interceptor Not Running

**Check provisioning:**
```bash
# Look for provisioning message in logs
grep "FEO Interceptor provisioned" <log-file>

# If message says "DISABLED", check environment variable
echo $ENABLE_FEO
```

**Verify module loaded:**
```bash
./frontend-development-proxy list-modules | grep feo
# Expected: http.handlers.feo_interceptor
```

### Data Not Merging

**Check CRD file path:**
```bash
# Verify file exists
ls -la config/deploy/frontend.yaml

# Check permissions
stat config/deploy/frontend.yaml
```

**Verify CRD schema format:**
The JavaScript bundle expects specific schema structure:
```yaml
spec:
  feoConfigEnabled: true
  navigationSegments: [...]    # Not "navigation.custom"
  bundleSegments: [...]
  searchEntries: [...]
  serviceTiles: [...]
  widgetRegistry: [...]
```

**Look for JavaScript errors:**
```bash
# Check for processing errors
grep -i "javascript\|processRequest" <log-file>

# Check for error header in response
curl -I http://localhost:1337/api/chrome-service/v1/static/bundles-generated.json | grep X-FEO
# If "X-FEO-Processing-Error: true", check logs
```

### File Changes Not Detected

**Verify file watcher started:**
```bash
grep "File watcher started" <log-file>
```

**Check fsnotify limits:**
```bash
# Linux: Check inotify watches limit
cat /proc/sys/fs/inotify/max_user_watches

# If too low, increase:
echo 524288 | sudo tee /proc/sys/fs/inotify/max_user_watches
```

**Test file watching manually:**
```bash
# 1. Start proxy with logs visible
./frontend-development-proxy

# 2. In another terminal, edit CRD
echo "# comment" >> config/deploy/frontend.yaml

# 3. Should see in logs:
# INFO  CRD file changed, reloading
# INFO  CRD file loaded successfully
```

### Performance Issues

**Check cache usage:**
```bash
# Enable debug logging
export CADDY_LOG_LEVEL=debug
./frontend-development-proxy

# Look for cache hits
grep "Using cached CRD file" <log-file>
```

**Verify file watching enabled:**
If every request shows file reads instead of cache hits, file watching may not be working.

**Expected overhead:**
- With file watching: ~5-10ms per intercepted request
- Without file watching: ~15-25ms per intercepted request (includes file I/O)

## Development

When making changes to the interceptor logic:

1. **Make changes in frontend-components:**
   Modify `packages/config-utils/src/feo/` in the frontend-components repository

2. **Build the bundle:**
   ```bash
   cd packages/config-utils
   npm run build:interceptors
   ```

3. **Test locally:**
   ```bash
   cp standalone/feo-interceptors.js /path/to/frontend-development-proxy/feo_interceptor/interceptors_bundled.js
   ```

4. **Publish new version:**
   Once stable, publish new version of `frontend-components-config-utilities`

5. **Update both proxies:**
   Update webpack-based and Caddy-based proxies to use new version

## Related

- Source: https://github.com/RedHatInsights/frontend-components/tree/master/packages/config-utils/src/feo
- Package: https://www.npmjs.com/package/@redhat-cloud-services/frontend-components-config-utilities
- Testing Guide: `../TESTING_GUIDE.md`
- Integration Test: `../test-feo-integration.sh`
