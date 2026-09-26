# terraform-provider-caddy

OpenTofu/Terraform provider that manages Caddy 2 through the Admin API.

## Stack

- Go 1.25+ (module path `github.com/coffedahl/terraform-provider-caddy`)
- terraform-plugin-framework (protocol 6)
- Caddy 2.8+ native JSON config; 2.10+ recommended for wildcard cert reuse

## Layout

- `internal/client` — Admin API (HTTP and unix socket), ETag retries, write mutex
- `internal/caddyjson` — HCL models compiled to Caddy JSON (`@id`, routes, handlers, TLS)
- `internal/provider` — provider, resources, data sources, acceptance tests
- `examples/` — copy-paste configs used by tfplugindocs
- `docs/` — generated; do not hand-edit

## Conventions

- HCL follows Caddyfile concepts (`caddy_site`, `handle`, `tls`), not a raw JSON dump
- Every managed object has `@id`; Terraform ID equals `@id`
- Import accepts live Caddyfile objects (`{server}/{index}`) and stamps `@id` when missing
- Host wildcards are a single leftmost label (`*.example.com`)
- Nested site handles become a `subroute`; site routes are `terminal` by default
- Automatic HTTPS is left on unless the user disables it; DNS-01 requires an xcaddy DNS module
- Mark DNS tokens, PEM material, and client keys `Sensitive`
- Do not import `github.com/caddyserver/caddy` — keep our JSON structs and golden tests instead
- Do not import terraform-plugin-sdk; use terraform-plugin-testing for acc tests

## Commands

```
make fmt
make test
make testacc    # needs TF_ACC=1 and a Caddy admin endpoint
make generate   # tfplugindocs
```

Acceptance tests honor `CADDY_ENDPOINT` (default `http://127.0.0.1:2019`).
