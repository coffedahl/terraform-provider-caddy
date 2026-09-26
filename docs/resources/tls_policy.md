---
page_title: "caddy_tls_policy Resource - caddy"
subcategory: ""
description: |-
  A Caddy TLS automation policy (apps.tls.automation.policies).
---

# Resource: caddy_tls_policy

A Caddy TLS automation policy (`apps.tls.automation.policies`). Use this for global on-demand TLS (`ask`) and default issuers. Site-level `tls` blocks create per-site policies automatically.

On-demand TLS must be restricted. Set `ask` to an internal HTTP endpoint; Caddy GETs `?domain=` and expects HTTP 200 to allow issuance.

## Example Usage

```terraform
resource "caddy_tls_policy" "on_demand" {
  name      = "on-demand"
  on_demand = true
  ask       = "http://127.0.0.1:5555/tls-allow"

  issuer {
    module = "acme"
    email  = "ops@example.com"
  }
}
```

## Schema

### Required

- `name` (String) Policy `@id`. Changing this forces replacement.

### Optional

- `ask` (String) On-demand permission endpoint.
- `issuer` (Block List) Certificate issuers (ACME, internal, ...).
- `on_demand` (Boolean) Enable on-demand TLS for these subjects.
- `subjects` (List of String) Hostnames this policy applies to. Empty means the default policy.

### Read-Only

- `id` (String) Caddy `@id`, equal to `name`.

## Import

```shell
tofu import caddy_tls_policy.wildcard 0
tofu import caddy_tls_policy.wildcard tls-apps-wildcard
```
