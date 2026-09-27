// Copyright (c) 2026 terraform-provider-caddy contributors
// SPDX-License-Identifier: MPL-2.0

package caddyjson

import (
	"encoding/json"
	"testing"
)

// as asserts v has type T, failing the test instead of panicking.
func as[T any](t *testing.T, v any) T {
	t.Helper()
	got, ok := v.(T)
	if !ok {
		t.Fatalf("expected %T, got %T (%v)", *new(T), v, v)
	}
	return got
}

func TestValidateHost(t *testing.T) {
	t.Parallel()
	ok := []string{"example.com", "foo.example.com", "*.example.com", "*"}
	for _, h := range ok {
		if err := ValidateHost(h); err != nil {
			t.Errorf("%s: %v", h, err)
		}
	}
	bad := []string{"", ".example.com", "example.com.", "foo.*.example.com", "foo*.example.com", "*.*.example.com"}
	for _, h := range bad {
		if err := ValidateHost(h); err == nil {
			t.Errorf("%s: expected error", h)
		}
	}
}

func TestCompileWildcardSite(t *testing.T) {
	t.Parallel()
	site := Site{
		Name:     "apps-wildcard",
		Hosts:    []string{"*.example.com"},
		Terminal: true,
		Handles: []Handle{
			{
				Match: &Match{Host: []string{"api.example.com"}, Path: []string{"/v1/*"}},
				ReverseProxy: &ReverseProxy{
					Upstreams: []Upstream{{Dial: "127.0.0.1:8080"}},
				},
			},
			{
				Match:      &Match{Path: []string{"/static/*"}},
				FileServer: &FileServer{Root: "/var/www/apps"},
			},
			{Abort: true},
		},
	}

	got, err := CompileSite(site)
	if err != nil {
		t.Fatal(err)
	}

	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["@id"] != "apps-wildcard" {
		t.Fatalf("@id = %v", decoded["@id"])
	}
	if decoded["terminal"] != true {
		t.Fatal("expected terminal")
	}
	match := as[map[string]any](t, as[[]any](t, decoded["match"])[0])
	hosts := as[[]any](t, match["host"])
	if hosts[0] != "*.example.com" {
		t.Fatalf("host = %v", hosts)
	}
	handle := as[map[string]any](t, as[[]any](t, decoded["handle"])[0])
	if handle["handler"] != "subroute" {
		t.Fatalf("handler = %v", handle["handler"])
	}
	routes := as[[]any](t, handle["routes"])
	if len(routes) != 3 {
		t.Fatalf("routes = %d", len(routes))
	}
	first := as[map[string]any](t, routes[0])
	if first["@id"] != "apps-wildcard__h0" {
		t.Fatalf("nested id = %v", first["@id"])
	}
	inner := as[map[string]any](t, as[[]any](t, first["handle"])[0])
	if inner["handler"] != "reverse_proxy" {
		t.Fatalf("inner handler = %v", inner["handler"])
	}
}

func TestCompileReverseProxyTLSSkipVerify(t *testing.T) {
	t.Parallel()
	h, err := compileReverseProxy(&ReverseProxy{
		Upstreams:             []Upstream{{Dial: "proxmox.home.lan:8006"}},
		TLSInsecureSkipVerify: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	transport := as[map[string]any](t, h["transport"])
	tls := as[map[string]any](t, transport["tls"])
	if tls["insecure_skip_verify"] != true {
		t.Fatalf("tls = %#v", tls)
	}
}

func TestCompileTLSPolicyDNS(t *testing.T) {
	t.Parallel()
	p := Policy{
		Name:     "tls-apps-wildcard",
		Subjects: []string{"*.example.com"},
		Issuers: []Issuer{{
			Module: "acme",
			Email:  "ops@example.com",
			DNS: &DNSProvider{
				Name:               "cloudflare",
				Config:             map[string]string{"api_token": "secret"},
				Resolvers:          []string{"1.1.1.1", "1.0.0.1"},
				PropagationDelay:   "5m",
				PropagationTimeout: "20m",
			},
		}},
	}
	got, err := CompileTLSPolicy(p)
	if err != nil {
		t.Fatal(err)
	}
	issuers := as[[]map[string]any](t, got["issuers"])
	challenges := as[map[string]any](t, issuers[0]["challenges"])
	dns := as[map[string]any](t, challenges["dns"])
	provider := as[map[string]any](t, dns["provider"])
	if provider["name"] != "cloudflare" {
		t.Fatalf("provider = %v", provider)
	}
	if provider["api_token"] != "secret" {
		t.Fatal("expected api_token passthrough")
	}
	if delay, _ := dns["propagation_delay"].(string); delay != "5m" {
		t.Fatalf("propagation_delay = %v", dns["propagation_delay"])
	}
}

func TestInsertRoutePriority(t *testing.T) {
	t.Parallel()
	routes := []map[string]any{
		{"@id": "a"},
		{"@id": "b"},
	}
	got := InsertRoute(routes, map[string]any{"@id": "apex"}, 0)
	if got[0]["@id"] != "apex" {
		t.Fatalf("got %v", got)
	}
	got = InsertRoute(got, map[string]any{"@id": "apex"}, 1)
	if got[1]["@id"] != "apex" {
		t.Fatalf("reinsert = %v", got)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d", len(got))
	}
}

func TestInsertRouteKeepsPositionWhenUnspecified(t *testing.T) {
	t.Parallel()
	routes := []map[string]any{
		{"@id": "a"},
		{"@id": "b"},
		{"@id": "c"},
	}
	got := InsertRoute(routes, map[string]any{"@id": "b", "n": 1}, PriorityUnspecified)
	if got[1]["@id"] != "b" || got[0]["@id"] != "a" || got[2]["@id"] != "c" {
		t.Fatalf("got %v", got)
	}
	got = InsertRoute(routes, map[string]any{"@id": "d"}, PriorityUnspecified)
	if got[3]["@id"] != "d" {
		t.Fatalf("append = %v", got)
	}
}

func TestCompileRedir(t *testing.T) {
	t.Parallel()
	h, err := compileRedir(&Redir{To: "https://example.com{http.request.uri}", StatusCode: 308})
	if err != nil {
		t.Fatal(err)
	}
	if h["handler"] != "static_response" {
		t.Fatal(h)
	}
	headers := as[map[string][]string](t, h["headers"])
	if headers["Location"][0] != "https://example.com{http.request.uri}" {
		t.Fatal(headers)
	}
}

func TestCompileAbort(t *testing.T) {
	t.Parallel()
	h, err := compileHandler(Handle{Abort: true})
	if err != nil {
		t.Fatal(err)
	}
	if h["handler"] != "static_response" || h["abort"] != true {
		t.Fatalf("got %#v", h)
	}
}

func TestCompileTryFiles(t *testing.T) {
	t.Parallel()
	got, err := CompileHandle("spa", 0, Handle{
		Rewrite: &Rewrite{TryFiles: []string{"{http.request.uri.path}", "/index.html"}},
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	match := as[[]map[string]any](t, got["match"])[0]
	file := as[map[string]any](t, match["file"])
	try := as[[]string](t, file["try_files"])
	if try[1] != "/index.html" {
		t.Fatal(try)
	}
}
