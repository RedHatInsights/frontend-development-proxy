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
    crd_path /config/deploy/frontend.yaml
}
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

### Troubleshooting

If the interceptor isn't working:

1. **Check the CRD file exists**: `deploy/frontend.yaml` must be present
2. **Verify feoConfigEnabled**: Must be set to `true` in the CRD
3. **Check logs**: Look for "FEO Interceptor provisioned" in container logs
4. **Verify URL patterns**: Only `*-generated.json` endpoints are intercepted

View logs:
```bash
podman logs <container-id>
```