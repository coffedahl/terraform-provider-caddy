---
page_title: "Caddy Provider"
description: |-
  Manage Caddy 2 through the Admin API: servers, wildcard sites, routes, reverse proxy, file serving, and TLS.
---

# Caddy Provider

The Caddy provider configures a running [Caddy](https://caddyserver.com/) 2 process using its [Admin API](https://caddyserver.com/docs/api). HCL follows Caddyfile concepts (site, handle, matchers, tls) and is compiled to Caddy JSON. Every managed object has an `@id`.

Wildcard hostnames (`*.example.com`) are first-class: nested `handle` blocks become a `subroute` so the wildcard host matcher stays at the top level, which is what Caddy needs for automatic HTTPS and wildcard certificates.

DNS-01 (required for wildcards) needs a Caddy build that includes the matching [caddy-dns](https://github.com/caddy-dns) module.

Use a unix socket admin endpoint in production. The provider does not install or start Caddy; persist API-applied config with `caddy run --resume`.

## Example Usage

```terraform
terraform {
  required_providers {
    caddy = {
      source = "coffedahl/caddy"
    }
  }
}

provider "caddy" {
  endpoint = "unix:///run/caddy/admin.sock"
}
```

## Schema

### Optional

- `endpoint` (String) Caddy Admin API address. Accepts `http://127.0.0.1:2019` (default), `https://...` for remote admin, or `unix:///run/caddy/admin.sock`. May also be set with `CADDY_ENDPOINT` or `CADDY_ADMIN`.
