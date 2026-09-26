# Live Caddyfile-style route (no @id): server key + 0-based route index.
tofu import caddy_site.apps srv0/0

# After import, or for a provider-managed site, the @id also works:
tofu import caddy_site.apps apps-wildcard
tofu import caddy_site.apps srv0/apps-wildcard
