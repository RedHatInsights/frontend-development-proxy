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

### Local startup modes

This repository now supports an explicit IOP toggle via `IOP`.

- Normal mode (default behavior): `npm run dev-proxy`
- IOP mode: `npm run IOP`
- Stop either mode: `npm run dev-proxy:down`

IOP mode is enabled only when `IOP` is exactly the string `true`.
When enabled, the proxy loads IOP-specific route overrides from
`config/custom_routes.iop.json` (or your `LOCAL_CUSTOM_ROUTES_PATH` override).
When disabled/unset, default behavior is unchanged and the proxy uses
`config/custom_routes.json`.
IOP mode also uses `Caddyfile.iop` while normal mode uses `Caddyfile`.

In IOP mode specifically:
- The fallback proxy uses TLS transport with `tls_insecure_skip_verify`.
- Generated local route `reverse_proxy` blocks omit `header_up Host {http.reverse_proxy.upstream.hostport}`.

### External launcher contract (fec dev-proxy)

This image can be launched externally (for example by
`@redhat-cloud-services/frontend-components-config` via `fec dev-proxy`).
The proxy itself only reads standard env vars and mounted files:

- `IOP` enables IOP mode only when exactly `true`.
- `ROUTES_JSON_PATH` points to main routes (default `/config/routes.json`).
- `LOCAL_CUSTOM_ROUTES_PATH` points to custom route overlay.

Expected IOP parity contract for external launchers:

- Env:
  - `IOP=true`
  - `LOCAL_CUSTOM_ROUTES_PATH=/config/custom_routes.iop.json`
- Mounts:
  - generated main routes -> `/config/routes.json`
  - optional IOP custom routes -> `/config/custom_routes.iop.json`

Important:

- `FEC_IOP_CUSTOM_ROUTES_PATH` is a caller-side variable used by `fec dev-proxy`
  to decide whether to mount a host file. This proxy image does not read
  `FEC_IOP_CUSTOM_ROUTES_PATH` directly.
- If the custom IOP file is not mounted/present, startup falls back to main routes
  and logs `Loading default routes only`.

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

#### IOP mode contract

- Env var name: `IOP`
- Type: boolean conveyed as string
- Enabled only when: `IOP=true` (exact match)
- Default: disabled (unset/any other value)

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

## Documentation

- [AGENTS.md](AGENTS.md) — AI agent onboarding guide, project conventions, and docs index
- [CONTRIBUTING.md](CONTRIBUTING.md) — How to contribute (commit conventions, code style, PR guidelines)
- [Architecture](docs/ARCHITECTURE.md) — System design, request flow, and key design decisions
- [Security Guidelines](docs/security-guidelines.md) — JWT handling, proxy security, container security
- [Testing Guidelines](docs/testing-guidelines.md) — Go module testing, integration test patterns