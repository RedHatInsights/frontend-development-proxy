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

### For App Developers (Using the Proxy)

Most users consume this proxy through `fec dev-proxy` from `@redhat-cloud-services/frontend-components-config`.

#### Normal Mode (Stage/Prod)

In your app's `package.json`:

```json
{
  "scripts": {
    "start:proxy": "PROXY=true fec dev"
  }
}
```

Then run: `npm run start:proxy`

#### IOP Mode

**1. Create a symlink in your app (one-time setup):**

IOP serves assets from `/assets/apps/` but webpack builds to `/dist/apps/`. Create a symlink:

```bash
mkdir -p dist/assets && ln -s ../apps dist/assets/apps
```

**2. Create module federation patch for IOP (if needed):**

IOP/Foreman hosts provide `react` in the module federation shared scope but not `react/jsx-runtime` or `react-intl`. If your app uses these, create `config/patchFederationForIop.js`:

```javascript
/* eslint-disable no-undef */
/**
 * Foreman/IOP hosts provide react in the module federation shared scope but not
 * react/jsx-runtime or react-intl. FEC defaults mark these as chromeProvided with
 * import: false, so the remote fails with:
 *   Shared module react/jsx-runtime doesn't exist in shared scope default
 *   (0 , react_intl__WEBPACK_IMPORTED_MODULE_0__.createIntlCache) is not a function
 *
 * Remove these from chromeProvided so webpack bundles them in the remote.
 */
const federatedModulesPath = require.resolve(
    '@redhat-cloud-services/frontend-components-config-utilities/federated-modules'
);
const federatedModulesUtil = require(federatedModulesPath);
const originalCreateIncludes = federatedModulesUtil.createIncludes;

federatedModulesUtil.createIncludes = (...args) => {
    const includes = originalCreateIncludes(...args);
    if (includes.chromeProvided) {
        delete includes.chromeProvided['react/jsx-runtime'];
        delete includes.chromeProvided['react-intl'];
    }
    return includes;
};
```

**3. Update `fec.config.js` in your app:**

```javascript
const { resolve } = require('path');

// Conditionally apply IOP federation patch
if (process.env.IOP === 'true') {
  require('./config/patchFederationForIop');
}

module.exports = {
  // ... other config
  ...(process.env.IOP === 'true' ? { deployment: 'assets/apps' } : { publicPath: 'auto' }),
  SPAFallback: process.env.IOP !== 'true',  // Disable chrome loading in IOP mode
  // ... rest of config
}
```

**4. Create `custom_routes.json` in your app root:**

```jsonc
{
  "/assets/apps/your-app/*": {
    "url": "http://host.docker.internal:8003"
  },
  "/api/your-app/*": {
    "url": "http://host.docker.internal:8000"
  }
}
```

**Note:** Port should match your webpack dev server port. The symlink from step 1 eliminates the need for `strip_prefix`.

**5. Add script to your `package.json`:**

```json
{
  "scripts": {
    "start:proxy:iop": "PROXY=true IOP=true HCC_ENV=iop HCC_ENV_URL=${IOP_URL} FEC_IOP_CUSTOM_ROUTES_PATH=$(pwd)/custom_routes.json fec dev-proxy --iop"
  }
}
```

**6. Run:**

```bash
export IOP_URL=https://your-iop-instance.example.com
npm run start:proxy:iop
```

Access at: `https://iop.foo.redhat.com:1337`

The published proxy image will be pulled automatically.

### For Proxy Developers (Working on This Repo)

**Build the image locally:**

```bash
podman build -t localhost/frontend-development-proxy:local .
```

**Use the local image in your app:**

```bash
export FEC_DEV_PROXY_IMAGE=localhost/frontend-development-proxy:local
export IOP_URL=https://your-iop-instance.example.com
npm run start:proxy:iop
```

This tells `fec dev-proxy` to use your local build instead of the published image.

**How IOP Mode Works:**

When `HCC_ENV` starts with "iop", the Caddyfile activates IOP-specific features:
- TLS certificate verification is disabled (`tls_insecure_skip_verify`)
- Location headers are rewritten for proper redirects

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

To disable all log output from the proxy (both Caddy access/runtime logs and
entrypoint messages), set `PROXY_LOGGING=false`:

```sh
podman run -d \
  -e PROXY_LOGGING=false \
  -p 1337:1337 \
  -v "$(pwd)/config:/config:ro,Z" \
  quay.io/redhat-user-workloads/hcc-platex-services-tenant/frontend-development-proxy:latest
```
#### IOP mode contract

- Env var name: `IOP`
- Type: boolean conveyed as string
- Enabled only when: `IOP=true` (exact match)
- Default: disabled (unset/any other value)

#### ⚠️ Security Warning: IOP Mode TLS Verification

**IOP mode disables TLS certificate verification (`tls_insecure_skip_verify`)** when proxying to the upstream IOP instance. This means:
- **Man-in-the-middle attacks are possible** — an attacker on your network could intercept traffic
- **Certificate errors are silently ignored** — expired, self-signed, or invalid certificates will be accepted
- **DO NOT use this mode on untrusted networks** (public WiFi, shared networks, etc.)

**Why this is necessary**: IOP instances often use self-signed certificates or internal CAs not trusted by the system store, making strict TLS verification impractical for local development.

**For production-like security**: Configure your system to trust the IOP instance's CA certificate instead of using this mode. See your IOP administrator for the CA certificate bundle.

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
