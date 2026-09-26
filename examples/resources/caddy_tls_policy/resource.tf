resource "caddy_tls_policy" "on_demand" {
  name      = "on-demand"
  on_demand = true
  ask       = "http://127.0.0.1:5555/tls-allow"

  issuer {
    module = "acme"
    email  = "ops@example.com"
  }
}
