terraform {
  required_providers {
    caddy = {
      source = "coffedahl/caddy"
    }
  }
}

# Prefer a unix socket in production.
provider "caddy" {
  endpoint = "unix:///run/caddy/admin.sock"
}
