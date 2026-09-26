resource "caddy_handle" "tenant" {
  for_each = var.tenants

  name     = "tenant-${each.key}"
  site_id  = caddy_site.apps.id
  priority = each.value.priority

  match {
    host = ["${each.key}.example.com"]
  }

  reverse_proxy {
    upstream {
      dial = each.value.upstream
    }
    transport_tls            = each.value.transport_tls
    tls_insecure_skip_verify = each.value.tls_insecure_skip_verify
  }
}
