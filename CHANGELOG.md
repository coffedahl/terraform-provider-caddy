# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org/).

## 0.1.0 (2026-09-27)

First release. This project is vibe-coded and not intended for production use without your own thorough audit; see the warning in the README.

### Resources

- `caddy_server`: HTTP server (`apps.http.servers`).
- `caddy_site`: host matcher with nested handles and optional TLS automation, including wildcard certificates through ACME DNS-01.
- `caddy_handle`: standalone handle attached to a site, for `for_each`.
- `caddy_tls_policy`: TLS automation policy, including on-demand TLS with `ask`.

### Data sources

- `caddy_config`: a slice of the live JSON config.
- `caddy_upstream_status`: reverse proxy upstream health.
- `caddy_inventory`: servers, sites and TLS policies on a live Caddy, with import IDs.

### Notes

- Import works on Caddyfile-generated config: objects without an `@id` are imported by index and get one assigned.
- Tested with OpenTofu 1.6 and later, and Caddy 2.8 and later.
