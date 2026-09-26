// Copyright (c) 2026 terraform-provider-caddy contributors
// SPDX-License-Identifier: MPL-2.0

package caddyjson

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// LiveServer is one apps.http.servers entry from a running Caddy.
type LiveServer struct {
	Name   string
	Listen []string
	Routes []LiveRoute
}

// LiveRoute is a top-level HTTP route (a site).
type LiveRoute struct {
	Index    int
	ID       string
	Hosts    []string
	Terminal bool
	Handles  []LiveHandle
}

// LiveHandle is a subroute entry under a site.
type LiveHandle struct {
	Index int
	ID    string
}

// LivePolicy is one TLS automation policy.
type LivePolicy struct {
	Index    int
	ID       string
	Subjects []string
	OnDemand bool
}

// Inventory is a snapshot of importable objects on a live Caddy.
type Inventory struct {
	Servers  []LiveServer
	Policies []LivePolicy
}

// ParseServersMap decodes apps.http.servers object.
func ParseServersMap(raw json.RawMessage) ([]LiveServer, error) {
	objs, err := DecodeObjectMap(raw)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(objs))
	for name := range objs {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]LiveServer, 0, len(objs))
	for _, name := range names {
		obj := objs[name]
		s := LiveServer{
			Name:   name,
			Listen: stringSlice(obj["listen"]),
		}
		for i, route := range objectList(obj["routes"]) {
			lr := LiveRoute{
				Index:    i,
				ID:       stringField(route, "@id"),
				Terminal: boolField(route, "terminal"),
			}
			if m := firstMatch(route); m != nil {
				lr.Hosts = stringSlice(m["host"])
			}
			for j, h := range siteHandleObjects(route) {
				lr.Handles = append(lr.Handles, LiveHandle{
					Index: j,
					ID:    stringField(h, "@id"),
				})
			}
			s.Routes = append(s.Routes, lr)
		}
		out = append(out, s)
	}
	return out, nil
}

// ParsePolicies decodes apps.tls.automation.policies.
func ParsePolicies(raw json.RawMessage) ([]LivePolicy, error) {
	list, err := DecodeObjectList(raw)
	if err != nil {
		return nil, err
	}
	out := make([]LivePolicy, 0, len(list))
	for i, obj := range list {
		out = append(out, LivePolicy{
			Index:    i,
			ID:       stringField(obj, "@id"),
			Subjects: stringSlice(obj["subjects"]),
			OnDemand: boolField(obj, "on_demand"),
		})
	}
	return out, nil
}

// ParsePolicy reconstructs a Policy from live JSON.
func ParsePolicy(obj map[string]any) Policy {
	p := Policy{
		Name:     stringField(obj, "@id"),
		Subjects: stringSlice(obj["subjects"]),
		OnDemand: boolField(obj, "on_demand"),
	}
	for _, iss := range objectList(obj["issuers"]) {
		p.Issuers = append(p.Issuers, parseIssuer(iss))
	}
	return p
}

func parseIssuer(obj map[string]any) Issuer {
	iss := Issuer{
		Module: stringField(obj, "module"),
		CA:     stringField(obj, "ca"),
		Email:  stringField(obj, "email"),
	}
	challenge := nestedMap(obj, "challenges", "dns")
	if challenge == nil {
		return iss
	}
	provider := nestedMap(challenge, "provider")
	if provider == nil {
		return iss
	}
	dns := &DNSProvider{
		Name:               stringField(provider, "name"),
		Config:             map[string]string{},
		Resolvers:          stringSlice(challenge["resolvers"]),
		PropagationDelay:   durationField(challenge, "propagation_delay"),
		PropagationTimeout: durationField(challenge, "propagation_timeout"),
		TTL:                durationField(challenge, "ttl"),
		OverrideDomain:     stringField(challenge, "override_domain"),
	}
	for k, v := range provider {
		if k == "name" {
			continue
		}
		if s, ok := v.(string); ok {
			dns.Config[k] = s
		}
	}
	iss.DNS = dns
	return iss
}

func durationField(obj map[string]any, key string) string {
	if obj == nil {
		return ""
	}
	switch t := obj[key].(type) {
	case string:
		return t
	case float64:
		// Caddy may emit durations as nanoseconds.
		if t == 0 {
			return ""
		}
		return fmt.Sprintf("%dns", int64(t))
	default:
		return ""
	}
}

// DecodeObjectMap decodes a JSON object of objects.
func DecodeObjectMap(raw json.RawMessage) (map[string]map[string]any, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return map[string]map[string]any{}, nil
	}
	var generic map[string]any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return nil, err
	}
	out := make(map[string]map[string]any, len(generic))
	for k, v := range generic {
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		out[k] = m
	}
	return out, nil
}

// DefaultSiteName picks a stable @id for a route that has none.
func DefaultSiteName(hosts []string, index int) string {
	if len(hosts) == 0 {
		return fmt.Sprintf("site-%d", index)
	}
	h := strings.ReplaceAll(hosts[0], "*", "wildcard")
	h = strings.Trim(h, ".")
	if h == "" {
		return fmt.Sprintf("site-%d", index)
	}
	return h
}

// UniqueID returns want, or want-2, want-3, ... until unused.
func UniqueID(want string, used map[string]struct{}) string {
	if _, ok := used[want]; !ok {
		return want
	}
	for i := 2; ; i++ {
		cand := fmt.Sprintf("%s-%d", want, i)
		if _, ok := used[cand]; !ok {
			return cand
		}
	}
}

// ParseIndex reports whether s is a non-negative integer.
func ParseIndex(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

func siteHandleObjects(route map[string]any) []map[string]any {
	if sub := SubrouteRoutes(route); len(sub) > 0 {
		return sub
	}
	handles := objectList(route["handle"])
	if len(handles) == 0 {
		return nil
	}
	if stringField(handles[0], "handler") == "subroute" {
		return nil
	}
	return []map[string]any{route}
}

// ImportCommands builds tofu import lines for a live inventory.
func ImportCommands(inv Inventory) []string {
	var out []string
	for _, srv := range inv.Servers {
		out = append(out, fmt.Sprintf("tofu import caddy_server.%s %s", sanitizeAddr(srv.Name), srv.Name))
		for _, rt := range srv.Routes {
			id := rt.ID
			if id == "" {
				id = strconv.Itoa(rt.Index)
			}
			addr := sanitizeAddr(DefaultSiteName(rt.Hosts, rt.Index))
			out = append(out, fmt.Sprintf("tofu import caddy_site.%s %s/%s", addr, srv.Name, id))
		}
	}
	for _, p := range inv.Policies {
		id := p.ID
		if id == "" {
			id = strconv.Itoa(p.Index)
		}
		addr := sanitizeAddr(id)
		if p.ID == "" && len(p.Subjects) > 0 {
			addr = sanitizeAddr(DefaultSiteName(p.Subjects, p.Index))
		}
		out = append(out, fmt.Sprintf("tofu import caddy_tls_policy.%s %s", addr, id))
	}
	return out
}

func sanitizeAddr(s string) string {
	s = strings.ReplaceAll(s, "*", "wildcard")
	s = strings.ReplaceAll(s, ".", "_")
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, "/", "_")
	if s == "" {
		return "imported"
	}
	if s[0] >= '0' && s[0] <= '9' {
		return "n" + s
	}
	return s
}
