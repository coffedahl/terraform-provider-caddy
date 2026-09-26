---
page_title: "caddy_site Resource - caddy"
subcategory: ""
description: |-
  A Caddy site: a top-level host matcher wrapping nested handles in a subroute.
---

# Resource: caddy_site

A Caddy site is a top-level host matcher wrapping nested handles in a `subroute`. Use `hosts = ["*.example.com"]` for a wildcard certificate covering one DNS label, then put per-path or per-subdomain routes in `handle` blocks.

Automatic HTTPS is used unless `tls` is set. Wildcard certificates require `tls.issuer.dns` and a Caddy build that includes that DNS module.

Exact hosts should use a lower `priority` than wildcards so they match first. Nested `handle` blocks keep declaration order inside the site.

## Example Usage

```terraform
resource "caddy_site" "apps" {
  name      = "apps-wildcard"
  server_id = caddy_server.https.id
  hosts     = ["*.example.com"]
  priority  = 1

  tls {
    issuer {
      module = "acme"
      email  = "ops@example.com"
      dns {
        name                = "cloudflare"
        resolvers           = ["1.1.1.1", "1.0.0.1"]
        propagation_delay   = "5m"
        propagation_timeout = "20m"
        config = {
          api_token = var.cloudflare_api_token
        }
      }
    }
  }

  handle {
    match {
      path = ["/api/*"]
    }
    reverse_proxy {
      upstream {
        dial = "127.0.0.1:8080"
      }
    }
  }

  handle {
    file_server {
      root = "/var/www"
    }
  }
}
```

## Schema

### Required

- `hosts` (List of String) Hostnames. `*.example.com` matches `foo.example.com` but not `foo.bar.example.com`.
- `name` (String) Stable `@id` for this site route. Changing this forces replacement.
- `server_id` (String) Name/`id` of the `caddy_server` this site is attached to.

### Optional

- `handle` (Block List) Ordered handlers. Set exactly one of `reverse_proxy`, `file_server`, `respond`, `redir`, `rewrite`, `header`, `encode`, `abort`, or `raw_json`.
- `priority` (Number) 0-based index among the server's routes. Put exact hosts before wildcards.
- `terminal` (Boolean) Stop evaluating later top-level routes on match. Defaults to true.
- `tls` (Block List, Max: 1) Optional TLS automation for this site's hosts.

### Read-Only

- `id` (String) Caddy `@id`, equal to `name`.

## Import

Caddyfile-managed routes usually have no `@id`. Import by server key and route index; the provider stamps an `@id` onto the live route.

```shell
tofu import caddy_site.apps srv0/0
tofu import caddy_site.apps srv0/apps-wildcard
tofu import caddy_site.apps apps-wildcard
```

Use `data.caddy_inventory.live` to list `import_id` values on a running instance.
