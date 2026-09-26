---
page_title: "caddy_upstream_status Data Source - caddy"
subcategory: ""
description: |-
  Live reverse_proxy upstream status from GET /reverse_proxy/upstreams.
---

# Data Source: caddy_upstream_status

Live reverse_proxy upstream status from `GET /reverse_proxy/upstreams`.

## Example Usage

```terraform
data "caddy_upstream_status" "live" {}

output "upstreams" {
  value = data.caddy_upstream_status.live.json
}
```

## Schema

### Read-Only

- `id` (String)
- `json` (String) JSON array of upstreams with `address`, `num_requests`, and `fails`.
