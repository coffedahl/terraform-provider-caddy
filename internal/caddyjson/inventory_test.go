// Copyright (c) 2026 terraform-provider-caddy contributors
// SPDX-License-Identifier: MPL-2.0

package caddyjson

import (
	"encoding/json"
	"testing"
)

func TestParseServersMapAndImportCommands(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`{
		"srv0": {
			"listen": [":443"],
			"routes": [
				{
					"match": [{"host": ["*.example.com"]}],
					"handle": [{"handler": "subroute", "routes": [
						{"handle": [{"handler": "file_server"}]}
					]}],
					"terminal": true
				}
			]
		}
	}`)
	servers, err := ParseServersMap(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 1 || servers[0].Name != "srv0" {
		t.Fatalf("%+v", servers)
	}
	if len(servers[0].Routes) != 1 || servers[0].Routes[0].Hosts[0] != "*.example.com" {
		t.Fatalf("route %+v", servers[0].Routes[0])
	}
	if servers[0].Routes[0].ID != "" {
		t.Fatal("expected empty @id")
	}
	cmds := ImportCommands(Inventory{Servers: servers})
	if len(cmds) < 2 {
		t.Fatalf("cmds %v", cmds)
	}
	if cmds[0] != "tofu import caddy_server.srv0 srv0" {
		t.Fatalf("got %q", cmds[0])
	}
	if cmds[1] != "tofu import caddy_site.wildcard_example_com srv0/0" {
		t.Fatalf("got %q", cmds[1])
	}
}

func TestDefaultSiteName(t *testing.T) {
	t.Parallel()
	if DefaultSiteName([]string{"*.example.com"}, 0) != "wildcard.example.com" {
		t.Fatal(DefaultSiteName([]string{"*.example.com"}, 0))
	}
	if DefaultSiteName(nil, 3) != "site-3" {
		t.Fatal(DefaultSiteName(nil, 3))
	}
}

func TestParsePolicy(t *testing.T) {
	t.Parallel()
	p := ParsePolicy(map[string]any{
		"@id":      "tls-apps",
		"subjects": []any{"*.example.com"},
		"issuers": []any{map[string]any{
			"module": "acme",
			"email":  "ops@example.com",
			"challenges": map[string]any{
				"dns": map[string]any{
					"provider": map[string]any{"name": "cloudflare", "api_token": "x"},
				},
			},
		}},
	})
	if p.Name != "tls-apps" || p.Issuers[0].DNS.Name != "cloudflare" {
		t.Fatalf("%+v", p)
	}
}
