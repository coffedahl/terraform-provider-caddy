data "caddy_upstream_status" "live" {}

output "upstreams" {
  value = data.caddy_upstream_status.live.json
}
