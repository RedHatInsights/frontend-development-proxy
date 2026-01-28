# Frontend Development Proxy

<!-- [![Static Badge](https://img.shields.io/badge/quay.io-dvagner%2Fconsoledot--testing--proxy-red)](https://quay.io/repository/dvagner/consoledot-testing-proxy) -->

Configurable container proxy for UI/E2E testing, implemented using the
[caddy](https://caddyserver.com/) proxy extended with a custom header transforming
extension.

## Usage

This container proxy will expose a [env | stage].foo.redhat.com:[port | 1337]
endpoint that will proxy request to routes defined by you and the rest to the
chosen consoledot environment. \

Usage (testing against stage):

```sh
podman run -d
  -e HTTPS_PROXY=$RH_STAGE_PROXY_URL
  -p 1337:1337
  -v "$(pwd)/config:/config:ro,Z"
  frontend-development-proxy quay.io/redhat-user-workloads/hcc-platex-services-tenant/frontend-development-proxy:latest
```

## Setup

All you really need is Podman or Docker and the app you want to test :)

### Hosts setup

In order to access the https://[env].foo.redhat.com in your browser, you have
to add entries to your /etc/hosts file. This is a one-time setup that has to
be done only once (unless you modify hosts) on each machine.

For each env you will need to add this to your `/etc/hosts` file, default env is
'stage':

```sh
127.0.0.1 [env].foo.redhat.com
::1 [env].foo.redhat.com
```

## Configuration

This proxy has configurable routes and the consoledot (HCC) environment

### Routes

The proxy can be configured for your apps/needs by providing a JSON configuration
file that defines the to-be proxied routes and a flag for endpoints that require
the RH identity header (APIs for example might, but not the static files), these
RH identity headers are automatically used for routes that start with `/api/`, you
can disable that by setting the `rh-identity-headers` flag to false.

This proxy is meant to be used along a locally running console app in the static
mode, i.e.: in your app run `npm|yarn fec static` or `npm start:federated` depending
on your setup. And if you want also your backend.

By default the container will expect the routes JSON config in `/config/routes.json`,
but if needed this can be changed by setting the `ROUTES_JSON_PATH` environment
variable. \

Example:

```jsonc
{
  // STATIC FILES FE
  "/apps/NAME-OF-YOUR-APP*": { "url": "http://host.docker.internal:8003" }, // this will proxy to a serer that runs on your machine at localhost:8003
  // YOUR BACKEND API
  "/api/NAME-OF-YOUR-APP/*": { "url": "http://host.docker.internal:8000" },
}
```

#### Using a locally running Chrome UI

For development with locally running Chrome UI
([insights-chrome](https://github.com/RedHatInsights/insights-chrome)) you need
to add a new route into the routes config that points to a server which serves
the chrome UI static files under the correct route and that has the `is_chrome`
flag set.

```jsonc
{
  "/apps/chrome*": {
    "url": "http://host.docker.internal:9912",
    "is_chrome": true, // this will enable the HTML fallback handle needed by the chrome UI
  },
}
```

You can start a server which will serve the chrome static files like so:

```sh
❯ npx http-server ./build -p 9912 -c-1 -a :: --cors=\*

# Beware that the build folder needs to have the static file in the
# `build/apps/chrome/` directory.
❯ ls build/apps/chrome/
.  ..  index.html  js

```

### Environment

The environment can be configured by setting the `HCC_ENV_URL` to something
different than the default of `console.stage.redhat.com` and all the uncatched
requests by your routes/matchers will be directed there.
Other than that you can also set `HCC_ENV` and `HCC_PORT` variables that just
change the exposed URL you are gonna be using.

For testing against stage you will also need to set the `HTTPS_PROXY` environment
variable to the RH stage proxy URL.

## DinD (docker-in-docker CI)

If your CI is a docker-in-docker setup, then there is a problem with using the
`host.docker.interal` addresses for targeting the services outside of the container.
This can be resolved by binding the container directly to the Podman/Docker host's
network, with no network isolation, which is why you might need to run it as root/superuser.
And changing the routes to `127.0.0.1`.

```sh
sudo podman run -d
  -e HTTPS_PROXY=$RH_STAGE_PROXY_URL
  -v "$(pwd)/config:/config:ro,Z"
  --network=host
  frontend-development-proxy quay.io/redhat-user-workloads/hcc-platex-services-tenant/frontend-development-proxy:latest
```

### Proxying Multiple URLs (Custom Routes)

You can route specific API paths to your local machine (or other targets) while letting the rest of the application use the default environment.

1.  **Create a config file** (e.g., `my-routes.json`):
    ```json
    {
        "/api/inventory": {
            "url": "[http://host.docker.internal:8000](http://host.docker.internal:8000)",
            "rh-identity-headers": true
        }
    }
    ```
    *Note: Use `host.docker.internal` to reach services running on your host machine.*

2.  **Run the proxy with the file mounted:**
    ```bash
    podman run -it --rm \
      -p 1337:1337 \
      -v $(pwd)/my-routes.json:/config/custom_routes.json:Z \
      quay.io/redhat-services/frontend-development-proxy:latest
    ```

## FEO (Frontend Environment Orchestrator) Interceptor

The proxy includes a built-in FEO interceptor that allows you to test local frontend configuration changes without deploying to a remote environment. This feature intercepts Chrome Service API responses and merges them with your local `deploy/frontend.yaml` CRD configuration.

> **Note**: The FEO interceptor logic is maintained in the [frontend-components](https://github.com/RedHatInsights/frontend-components) repository and distributed via npm (`@redhat-cloud-services/frontend-components-config-utilities`). This ensures consistent behavior across all proxies.

### What Does FEO Interceptor Do?

The FEO interceptor enables local testing of:

- **Navigation bundles** - Merge local nav items with remote navigation
- **Module federation** - Register local apps in the federated module registry
- **Search index** - Add local search entries to the search index
- **Service tiles** - Include local service tiles in categorized structure
- **Widget registry** - Register local widgets

### Supported API Endpoints

The interceptor automatically handles these Chrome Service API endpoints:

- `/api/chrome-service/v1/static/bundles-generated.json` (navigation)
- `/api/chrome-service/v1/static/fed-modules-generated.json` (module federation)
- `/api/chrome-service/v1/static/search-index-generated.json` (search)
- `/api/chrome-service/v1/static/service-tiles-generated.json` (service tiles)
- `/api/chrome-service/v1/static/widget-registry-generated.json` (widgets)

### Configuration

The FEO interceptor is configured in the `Caddyfile` and is enabled by default when a CRD file is present:

```caddyfile
feo_interceptor {
    crd_path config/deploy/frontend.yaml
}
```

### Environment Variables

The FEO interceptor can be controlled via environment variables:

- **`ENABLE_FEO`** (default: `true`) - Enable/disable FEO interceptor
  - Set to `false` or `0` to disable interception completely
  - Set to `true` or `1` to enable interception (default behavior)
  - Example: `ENABLE_FEO=false ./frontend-development-proxy`

- **`FEO_CRD_PATH`** (default: `config/deploy/frontend.yaml`) - Path to Frontend CRD file
  - Can be absolute or relative path
  - File is watched for changes and automatically reloaded
  - Example: `FEO_CRD_PATH=/custom/path/frontend.yaml`

### File Watching & Auto-Reload

The FEO interceptor automatically watches your CRD file for changes using `fsnotify`. When you edit the CRD file:

1. **Changes detected immediately** - File system events trigger reload
2. **File re-parsed and cached** - YAML parsed and stored in memory
3. **Next request uses updated config** - No need to restart the proxy
4. **Zero downtime** - Requests continue processing during reload

**Example workflow:**
```bash
# 1. Start proxy
./frontend-development-proxy

# 2. Edit CRD file while proxy is running
vim config/deploy/frontend.yaml
# Change navigation title to "Updated Feature"

# 3. Make request - sees updated data immediately
curl http://localhost:1337/api/chrome-service/v1/static/bundles-generated.json
```

Logs will show:
```
INFO  CRD file changed, reloading  {"path": "config/deploy/frontend.yaml"}
INFO  CRD file loaded successfully {"size": 1234}
```

### CRD File Format

Create a `deploy/frontend.yaml` file with your local configuration:

```yaml
apiVersion: v1
kind: List
objects:
  - apiVersion: console.redhat.com/v1
    kind: ConsoleFrontend
    metadata:
      name: my-app
    spec:
      feoConfigEnabled: true
      
      # Module configuration for fed-modules
      module:
        manifestLocation: /apps/my-app/fed-mods.json
        defaultDocumentTitle: "My App"
      
      # Frontend paths
      frontend:
        paths:
          - /apps/my-app
      
      # Navigation segments
      navigationSegments:
        - segmentId: my-app-nav
          navItems:
            - id: my-dashboard
              title: "My Dashboard"
              href: "/my-app/dashboard"
              description: "Local dashboard for testing"
      
      # Bundle segments (positions in navigation)
      bundleSegments:
        - segmentId: my-app-nav
          bundleId: insights
          position: 10
          navItems:
            - id: my-feature
              title: "My Feature"
              href: "/my-app/feature"
      
      # Search entries
      searchEntries:
        - title: "My Feature"
          bundleTitle: "My App"
          description: "Test feature from local dev"
          pathname: "/my-app/feature"
          frontendRef: my-app
      
      # Service tiles
      serviceTiles:
        - section: insights
          group: featured
          title: "My Service"
          description: "Local test service"
          pathname: "/my-app/service"
          icon: "rocket"
          frontendRef: my-app
      
      # Widget registry
      widgetRegistry:
        - widgetType: "my-widget"
          title: "My Widget"
          frontendRef: my-app
```

### Usage

#### Container Deployment

1. **Create your CRD file** from the example template:
   ```bash
   cp deploy/frontend.example.yaml deploy/frontend.yaml
   # Edit deploy/frontend.yaml with your app configuration
   ```

2. **Mount the deploy directory** when running the container:

```bash
podman run -it --rm \
  -p 1337:1337 \
  -v $(pwd)/deploy:/config/deploy:Z \
  quay.io/redhat-user-workloads/hcc-platex-services-tenant/frontend-development-proxy:latest
```

3. **Test your changes** - The proxy will automatically merge your local configuration with remote Chrome Service responses

#### Local Development

1. **Create your CRD file** from the example template:
   ```bash
   cp config/deploy/frontend.example.yaml config/deploy/frontend.yaml
   # Edit config/deploy/frontend.yaml with your app configuration
   ```

2. **Run the proxy** directly:
   ```bash
   export HCC_ENV=stage
   export ENABLE_FEO=true
   ./frontend-development-proxy run --config Caddyfile --adapter caddyfile
   ```

3. **Test your changes** - The proxy will automatically merge your local configuration with remote Chrome Service responses

### How It Works

1. Request arrives for a Chrome Service API endpoint
2. FEO interceptor captures the upstream response
3. Reads your local `deploy/frontend.yaml` CRD file
4. Merges local configuration with remote data
5. Returns the combined result to your application

### Example Output

If your remote navigation has:
```json
[
  {"title": "Remote Dashboard", "href": "/remote-dashboard"}
]
```

And your CRD defines:
```yaml
navItems:
  - title: "Local Feature"
    href: "/local-feature"
```

The interceptor returns:
```json
[
  {"title": "Remote Dashboard", "href": "/remote-dashboard"},
  {"title": "Local Feature", "href": "/local-feature"}
]
```

### Performance

- **Overhead**: ~10-15ms per intercepted request
- **Concurrency**: Thread-safe, supports unlimited parallel requests
- **Memory**: Isolated per request, automatically garbage collected

### Debugging

Enable detailed logging to see FEO interceptor activity:

**Key log messages to look for:**

```
INFO  FEO Interceptor provisioned           # Module loaded successfully
INFO  File watcher started                  # CRD file monitoring active
INFO  FEO intercepting request              # Request matched for interception
INFO  [JavaScript]                          # Messages from JavaScript processing
INFO  CRD file changed, reloading           # File watcher detected change
DEBUG Using cached CRD file                 # Using cached CRD data
```

**Error indicators:**

If you see `X-FEO-Processing-Error: true` in response headers (check browser DevTools), the interceptor encountered an error. Check logs for details.

### Troubleshooting

#### Interceptor Not Running

**Symptom:** No "FEO intercepting request" messages in logs

**Solutions:**
1. Check `ENABLE_FEO` environment variable is not set to `false`
2. Verify Caddyfile has `order feo_interceptor before reverse_proxy` in global section
3. Check proxy logs for "FEO Interceptor provisioned" message
4. Verify requests match intercepted endpoints (must end with `*-generated.json`)

#### Data Not Merging

**Symptom:** Response contains only remote data, no local CRD items

**Solutions:**
1. **Check CRD path:** Verify `crd_path` in Caddyfile points to existing file
   ```bash
   ls -la config/deploy/frontend.yaml
   ```

2. **Verify CRD schema:** Ensure CRD uses correct structure (`navigationSegments`, not `navigation.custom`)
   ```yaml
   spec:
     feoConfigEnabled: true
     navigationSegments:  # ← Must use this structure
       - segmentId: ...
   ```

3. **Check for JavaScript errors:** Look for error logs containing "JavaScript" or "processRequest"
   ```bash
   grep -i "javascript\|processRequest" <log-file>
   ```

4. **Verify response headers:** Check for `X-FEO-Processing-Error: true` header indicating processing failure

#### File Changes Not Detected

**Symptom:** Editing CRD file doesn't update responses

**Solutions:**
1. **Check file watcher started:** Look for "File watcher started" in logs
2. **Verify file permissions:** Ensure proxy can read the CRD file
3. **Check fsnotify:** Verify the file system supports inotify events
   ```bash
   cat /proc/sys/fs/inotify/max_user_watches
   ```

#### Performance Issues

**Symptom:** Slow response times for intercepted requests

**Expected overhead:** ~10-15ms per intercepted request (file watching reduces overhead by caching)

If significantly slower:
1. Check CRD file size (should be < 100KB)
2. Verify file watching is enabled (should see "Using cached CRD file" in debug logs)
3. Check for frequent file reloads (should only happen when CRD changes)

View logs:
```bash
# Container logs
podman logs <container-id>

# Or direct binary logs
./frontend-development-proxy 2>&1 | grep -i feo
```