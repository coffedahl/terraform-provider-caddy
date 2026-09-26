data "caddy_config" "full" {
  path = "/config/"
}

output "caddy_json" {
  value = data.caddy_config.full.json
}
