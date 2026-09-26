resource "caddy_server" "https" {
  name   = "https"
  listen = [":443"]
}
