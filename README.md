# terraform-provider-caddy

> [!WARNING]
> **This project is vibe-coded and is not intended for production use.**
>
> Most of this provider was written with AI assistance, and it has not had an independent security or code review. It changes and deletes live Caddy configuration through the Admin API, so a bug can take your sites offline or change how TLS certificates are issued.
>
> If you use it anyway, audit the code yourself first, test it against a disposable Caddy instance, and keep backups of your Caddy config. It comes with no warranty; see the [license](LICENSE).

OpenTofu/Terraform provider for [Caddy 2](https://caddyserver.com/). It manages a running Caddy instance through the [Admin API](https://caddyserver.com/docs/api), compiling HCL that looks like a Caddyfile into Caddy's native JSON config.

Use it to declare HTTP servers, wildcard (and ordinary) sites, nested routes, reverse proxies, file serving, and TLS — including DNS-01 for wildcard certificates.

## Requirements

- OpenTofu >= 1.6 (tested in CI with 1.6 and the latest release). Terraform >= 1.5 should work too, but is not tested.
- Caddy 2.8+ with the Admin API enabled (tested with 2.8 and the latest 2.x; 2.10+ recommended)
- For wildcard certificates: Caddy built with a [caddy-dns](https://github.com/caddy-dns) module (`xcaddy build --with github.com/caddy-dns/<provider>`)

## Persistence

The Admin API writes the live JSON to Caddy's autosave file when persist is on (the default). **A Caddyfile start/reload replaces that JSON.** After OpenTofu owns routes, do not `caddy reload` a Caddyfile that still defines those sites.

Use one of:

```bash
caddy run --resume
# or a systemd unit whose ExecStart includes --resume
# and a Caddyfile that only sets admin/globals, not the sites tofu manages
```

Confirm persist is on:

```bash
curl -s http://127.0.0.1:2019/config/admin | jq
```

`priority` on `caddy_site` / `caddy_handle` is only an insert index. **Omit it** so existing order is kept. If every handle is `priority = 0`, each apply moves every route to the front and the next plan tries to set them all to 0 again.

## Example

```hcl
terraform {
  required_providers {
    caddy = {
      source  = "coffedahl/caddy"
      version = "~> 0.1"
    }
  }
}

provider "caddy" {
  endpoint = "unix:///run/caddy/admin.sock"
}

resource "caddy_server" "https" {
  name   = "https"
  listen = [":443"]
}

resource "caddy_site" "apps" {
  name      = "apps-wildcard"
  server_id = caddy_server.https.id
  hosts     = ["*.example.com"]

  tls {
    issuer {
      module = "acme"
      email  = "ops@example.com"
      dns {
        name = "cloudflare"
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

Exact hosts should use a lower `priority` than wildcards so they match first. Nested `handle` blocks keep declaration order inside the site.

## Admin API

Caddy must already be running. The provider does not install or start Caddy.

| Endpoint | When |
|---|---|
| `unix:///run/caddy/admin.sock` | Production (recommended) |
| `http://127.0.0.1:2019` | Local default |
| `https://admin.example.com` | Admin API behind your own TLS reverse proxy |

Caddy's built-in remote admin (`admin.remote`, mutual TLS) is not supported yet: the provider cannot present a client certificate.

Set `CADDY_ENDPOINT` or `CADDY_ADMIN` instead of the provider argument if you prefer.

Persist API-applied config across restarts with `caddy run --resume`.

## Import a live Caddy

Caddyfile configs typically have no `@id`. Point the provider at the Admin API, then:

```hcl
data "caddy_inventory" "live" {}

output "import_commands" {
  value = data.caddy_inventory.live.import_commands
}
```

```shell
tofu import caddy_server.https srv0
tofu import caddy_site.apps srv0/0          # server / route index
tofu import caddy_tls_policy.wildcard 0
```

Importing a site by `{server}/{index}` assigns an `@id` on the live route so later applies can find it. Import IDs:

| Resource | ID |
|---|---|
| `caddy_server` | server key (`srv0`) |
| `caddy_site` | `{server}/{index\|@id}` or `{@id}` |
| `caddy_handle` | `{server}/{site}/{handle}` or `{@id}` |
| `caddy_tls_policy` | `{index\|@id}` |

## Resources

| Name | Purpose |
|---|---|
| `caddy_server` | HTTP server / listener |
| `caddy_site` | Host matcher + nested handles + optional TLS |
| `caddy_handle` | Extra handle on a site (`for_each`) |
| `caddy_tls_policy` | Global TLS automation / on-demand `ask` |

Data sources: `caddy_config`, `caddy_upstream_status`, `caddy_inventory`.

## TLS

- Ordinary public names: omit `tls` and let Caddy automatic HTTPS run
- Wildcards: DNS-01 via `tls.issuer.dns` (stock Caddy cannot do this)
- Internal names: `tls { internal = true }`
- On-demand: `tls { on_demand = true }` plus `caddy_tls_policy.ask`

Never point production at the Let's Encrypt production CA from a test loop. Use the [staging directory](https://letsencrypt.org/docs/staging-environment/) while iterating.

## Development

```
make test        # unit tests
make lint
make generate    # regenerate docs/ from templates/, examples/ and the schema
```

Acceptance tests need a disposable Caddy. They create and delete real config, so never point them at a production instance:

```
docker run -d --rm --name caddy-acc -p 127.0.0.1:2019:2019 -e CADDY_ADMIN=0.0.0.0:2019 caddy:2
TF_ACC=1 CADDY_ENDPOINT=http://127.0.0.1:2019 make testacc
```

With `TF_ACC` set, the tests fail rather than skip when the Admin API is unreachable. `TestAccSiteCertificateFile` also needs `CADDY_TEST_CERT_DIR` pointing at a directory, as Caddy sees it, containing `cert.pem` and `key.pem`.

`docs/` is generated. Edit `templates/`, `examples/` or the schema descriptions and run `make generate`.

## Releasing

The OpenTofu registry indexes GitHub releases of `coffedahl/terraform-provider-caddy`. Pushing a `vX.Y.Z` tag there runs `.github/workflows/release.yml`, which builds, checksums, signs and publishes the release with GoReleaser; the registry picks up new tags on its own. Published versions are immutable, so fix mistakes with a new version.

1. Add the change to `CHANGELOG.md`.
2. `git tag v0.1.0 && git push origin v0.1.0` (to GitHub).

Signing needs the `GPG_PRIVATE_KEY` and `PASSPHRASE` repository secrets, and the public key must be registered with the [OpenTofu registry](https://github.com/opentofu/registry/issues/new?template=provider_key.yml).

## License

Mozilla Public License 2.0
