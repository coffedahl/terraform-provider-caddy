resource "caddy_server" "https" {
  name   = "https"
  listen = [":443"]
}

resource "caddy_site" "apex" {
  name      = "apex"
  server_id = caddy_server.https.id
  hosts     = ["example.com"]
  priority  = 0

  handle {
    redir {
      to          = "https://www.example.com{http.request.uri}"
      status_code = 308
    }
  }
}

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
      host = ["api.example.com"]
      path = ["/v1/*"]
    }
    reverse_proxy {
      upstream {
        dial = "127.0.0.1:8080"
      }
    }
  }

  handle {
    match {
      path = ["/static/*"]
    }
    file_server {
      root = "/var/www/apps"
    }
  }

  handle {
    abort = true
  }
}
