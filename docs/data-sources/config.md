---
page_title: "caddy_config Data Source - caddy"
subcategory: ""
description: |-
  Read a slice of the live Caddy JSON config.
---

# Data Source: caddy_config

Read a slice of the live Caddy JSON config. Useful for debugging and drift inspection.

## Example Usage

```terraform
data "caddy_config" "full" {
  path = "/config/"
}

output "caddy_json" {
  value = data.caddy_config.full.json
}
```

## Schema

### Optional

- `path` (String) Admin API path, default `/config/`.

### Read-Only

- `id` (String) Same as `path`.
- `json` (String) Pretty-printed JSON at that path.
