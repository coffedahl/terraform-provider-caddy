---
page_title: "caddy_handle Resource - caddy"
subcategory: ""
description: |-
  A standalone handle attached to a caddy_site subroute.
---

# Resource: caddy_handle

A standalone handle attached to a `caddy_site` subroute. Use this with `for_each` when routes are dynamic. Nested `handle` blocks on `caddy_site` stay under `{site}__h*` ids and are not overwritten.

## Example Usage

```terraform
resource "caddy_handle" "tenant" {
  for_each = var.tenants

  name     = "tenant-${each.key}"
  site_id  = caddy_site.apps.id
  priority = each.value.priority

  match {
    host = ["${each.key}.example.com"]
  }

  reverse_proxy {
    upstream {
      dial = each.value.upstream
    }
  }
}
```

## Schema

### Required

- `name` (String) Stable `@id`. Must not use the `{site}__h*` prefix reserved for nested site handles.
- `site_id` (String) `name`/`id` of the parent `caddy_site`.

### Optional

- `abort` (Boolean) Abort the request (Caddyfile `abort` → `static_response.abort`).
- `encode` (Block List, Max: 1)
- `file_server` (Block List, Max: 1)
- `header` (Block List, Max: 1)
- `match` (Block List, Max: 1)
- `priority` (Number) Optional insert index. **Omit it** unless you are deliberately reordering. Setting every handle to `0` causes a plan loop.
- `raw_json` (String) Escape hatch JSON handler object.
- `redir` (Block List, Max: 1)
- `respond` (Block List, Max: 1)
- `reverse_proxy` (Block List, Max: 1)
- `rewrite` (Block List, Max: 1)

### Read-Only

- `id` (String) Caddy `@id`, equal to `name`.

## Import

```shell
tofu import caddy_handle.api srv0/0/1
tofu import caddy_handle.api tenant-acme
```

Prefer importing the parent `caddy_site` when handles should stay nested on that site.

## Reverse proxy TLS (upstream)

Caddyfile:

```caddyfile
reverse_proxy proxmox.home.lan:8006 {
    transport http {
        tls
        tls_insecure_skip_verify
    }
}
```

OpenTofu:

```hcl
reverse_proxy {
  upstream {
    dial = "proxmox.home.lan:8006"
  }
  transport_tls            = true
  tls_insecure_skip_verify = true
}
```

`tls_insecure_skip_verify` implies TLS to the upstream. Optional: `tls_server_name`, `tls_client_certificate_file`, `tls_client_certificate_key_file`.
