data "caddy_inventory" "live" {}

output "import_commands" {
  value = data.caddy_inventory.live.import_commands
}
