---
page_title: "caddy_inventory Data Source - caddy"
subcategory: ""
description: |-
  Snapshot of a live Caddy instance for tofu import.
---

# Data Source: caddy_inventory

Lists HTTP servers, top-level routes (sites), and TLS policies on a running Caddy. Use `import_commands` or each route's `import_id` with `tofu import`.

Routes that came from a Caddyfile usually have no `@id`. Importing a site with `{server}/{index}` stamps an `@id` onto the live route so later applies can find it.

## Example Usage

```terraform
data "caddy_inventory" "live" {}

output "import_commands" {
  value = data.caddy_inventory.live.import_commands
}
```

```shell
tofu import caddy_server.https srv0
tofu import caddy_site.apps srv0/0
tofu import caddy_tls_policy.wildcard 0
```

OpenTofu 1.5+ import blocks:

```terraform
import {
  to = caddy_server.https
  id = "srv0"
}

import {
  to = caddy_site.apps
  id = "srv0/0"
}
```

## Schema

### Read-Only

- `id` (String)
- `import_commands` (List of String) Suggested `tofu import ...` lines.
- `servers` (List of Object) HTTP servers. Each route has `index`, `id`, `hosts`, `handle_count`, and `import_id`.
- `tls_policies` (List of Object) TLS automation policies with `import_id`.
