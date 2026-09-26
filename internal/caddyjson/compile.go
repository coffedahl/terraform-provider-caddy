// Copyright (c) 2026 terraform-provider-caddy contributors
// SPDX-License-Identifier: MPL-2.0

package caddyjson

import (
	"encoding/json"
	"fmt"
	"strings"
)

// CompileServer returns the JSON object for apps.http.servers.{name}.
func CompileServer(s Server) (map[string]any, error) {
	if strings.TrimSpace(s.Name) == "" {
		return nil, fmt.Errorf("server name is required")
	}
	if len(s.Listen) == 0 {
		return nil, fmt.Errorf("server %q: listen is required", s.Name)
	}
	out := map[string]any{
		"@id":    s.Name,
		"listen": s.Listen,
	}
	if len(s.Protocols) > 0 {
		out["protocols"] = s.Protocols
	}
	if s.DisableAutoHTTPS || s.DisableRedirects || s.DisableCerts {
		auto := map[string]any{}
		if s.DisableAutoHTTPS {
			auto["disable"] = true
		}
		if s.DisableRedirects {
			auto["disable_redirects"] = true
		}
		if s.DisableCerts {
			auto["disable_certificates"] = true
		}
		out["automatic_https"] = auto
	}
	return out, nil
}

// CompileSite returns a top-level HTTP route for the site.
// Nested handles are wrapped in a subroute so the host matcher stays
// at the top level (required for automatic HTTPS and wildcard certs).
func CompileSite(s Site) (map[string]any, error) {
	if strings.TrimSpace(s.Name) == "" {
		return nil, fmt.Errorf("site name is required")
	}
	if err := ValidateHosts(s.Hosts); err != nil {
		return nil, fmt.Errorf("site %q: %w", s.Name, err)
	}

	handles, err := compileHandleList(s.Name, s.Handles, true)
	if err != nil {
		return nil, fmt.Errorf("site %q: %w", s.Name, err)
	}

	route := map[string]any{
		"@id": s.Name,
		"match": []map[string]any{
			{"host": s.Hosts},
		},
		"handle": []map[string]any{
			{
				"handler": "subroute",
				"routes":  handles,
			},
		},
		"terminal": s.Terminal,
	}
	return route, nil
}

// CompileHandle returns a single subroute route object.
func CompileHandle(siteName string, index int, h Handle, nested bool) (map[string]any, error) {
	id := h.ID
	if id == "" && nested {
		id = NestedHandleID(siteName, index)
	}
	if id == "" {
		return nil, fmt.Errorf("handle is missing a name/@id")
	}

	handler, err := compileHandler(h)
	if err != nil {
		return nil, err
	}

	route := map[string]any{
		"@id":    id,
		"handle": []map[string]any{handler},
	}
	m := compileMatch(h.Match)
	if h.Rewrite != nil && len(h.Rewrite.TryFiles) > 0 {
		if m == nil {
			m = map[string]any{}
		}
		m["file"] = map[string]any{"try_files": h.Rewrite.TryFiles}
	}
	if m != nil {
		route["match"] = []map[string]any{m}
	}
	return route, nil
}

func compileHandleList(siteName string, handles []Handle, nested bool) ([]map[string]any, error) {
	out := make([]map[string]any, 0, len(handles))
	for i, h := range handles {
		compiled, err := CompileHandle(siteName, i, h, nested)
		if err != nil {
			return nil, fmt.Errorf("handle %d: %w", i, err)
		}
		out = append(out, compiled)
	}
	return out, nil
}

func compileMatch(m *Match) map[string]any {
	if m == nil {
		return nil
	}
	out := map[string]any{}
	if len(m.Host) > 0 {
		out["host"] = m.Host
	}
	if len(m.Path) > 0 {
		out["path"] = m.Path
	}
	if len(m.Method) > 0 {
		out["method"] = m.Method
	}
	if len(m.Header) > 0 {
		out["header"] = m.Header
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func compileHandler(h Handle) (map[string]any, error) {
	count := 0
	if h.ReverseProxy != nil {
		count++
	}
	if h.FileServer != nil {
		count++
	}
	if h.Respond != nil {
		count++
	}
	if h.Redir != nil {
		count++
	}
	if h.Rewrite != nil {
		count++
	}
	if h.Header != nil {
		count++
	}
	if h.Encode != nil {
		count++
	}
	if h.Abort {
		count++
	}
	if strings.TrimSpace(h.RawJSON) != "" {
		count++
	}
	if count == 0 {
		return nil, fmt.Errorf("exactly one handler is required")
	}
	if count > 1 {
		return nil, fmt.Errorf("only one handler may be set per handle")
	}

	switch {
	case h.ReverseProxy != nil:
		return compileReverseProxy(h.ReverseProxy)
	case h.FileServer != nil:
		return compileFileServer(h.FileServer)
	case h.Respond != nil:
		return compileRespond(h.Respond)
	case h.Redir != nil:
		return compileRedir(h.Redir)
	case h.Rewrite != nil:
		return compileRewrite(h.Rewrite)
	case h.Header != nil:
		return compileHeader(h.Header)
	case h.Encode != nil:
		return compileEncode(h.Encode)
	case h.Abort:
		// Caddyfile `abort` adapts to static_response with abort:true
		// (there is no http.handlers.abort module in Caddy 2.10).
		return map[string]any{"handler": "static_response", "abort": true}, nil
	default:
		var raw map[string]any
		if err := json.Unmarshal([]byte(h.RawJSON), &raw); err != nil {
			return nil, fmt.Errorf("raw_json: %w", err)
		}
		if _, ok := raw["handler"]; !ok {
			return nil, fmt.Errorf("raw_json must include a handler field")
		}
		return raw, nil
	}
}

func compileReverseProxy(p *ReverseProxy) (map[string]any, error) {
	if len(p.Upstreams) == 0 {
		return nil, fmt.Errorf("reverse_proxy requires at least one upstream")
	}
	upstreams := make([]map[string]any, 0, len(p.Upstreams))
	for _, u := range p.Upstreams {
		if strings.TrimSpace(u.Dial) == "" {
			return nil, fmt.Errorf("reverse_proxy upstream dial is required")
		}
		upstreams = append(upstreams, map[string]any{"dial": u.Dial})
	}
	out := map[string]any{
		"handler":   "reverse_proxy",
		"upstreams": upstreams,
	}
	if len(p.HeaderUp) > 0 || len(p.HeaderDown) > 0 {
		headers := map[string]any{}
		if len(p.HeaderUp) > 0 {
			headers["request"] = map[string]any{"set": p.HeaderUp}
		}
		if len(p.HeaderDown) > 0 {
			headers["response"] = map[string]any{"set": p.HeaderDown}
		}
		out["headers"] = headers
	}
	if p.TransportTLS || p.TLSInsecureSkipVerify || p.TLSServerName != "" || p.TLSClientCertFile != "" {
		tls := map[string]any{}
		if p.TLSInsecureSkipVerify {
			tls["insecure_skip_verify"] = true
		}
		if p.TLSServerName != "" {
			tls["server_name"] = p.TLSServerName
		}
		if p.TLSClientCertFile != "" {
			tls["client_certificate_file"] = p.TLSClientCertFile
		}
		if p.TLSClientKeyFile != "" {
			tls["client_certificate_key_file"] = p.TLSClientKeyFile
		}
		out["transport"] = map[string]any{
			"protocol": "http",
			"tls":      tls,
		}
	}
	if p.Selection != "" {
		out["load_balancing"] = map[string]any{
			"selection_policy": map[string]any{"policy": p.Selection},
		}
	}
	if p.HealthURI != "" {
		active := map[string]any{"uri": p.HealthURI}
		if p.HealthInterval != "" {
			active["interval"] = p.HealthInterval
		}
		out["health_checks"] = map[string]any{"active": active}
	}
	return out, nil
}

func compileFileServer(f *FileServer) (map[string]any, error) {
	out := map[string]any{"handler": "file_server"}
	if f.Root != "" {
		out["root"] = f.Root
	}
	if f.Browse {
		out["browse"] = map[string]any{}
	}
	if len(f.Hide) > 0 {
		out["hide"] = f.Hide
	}
	if len(f.IndexNames) > 0 {
		out["index_names"] = f.IndexNames
	}
	if len(f.Precompressed) > 0 {
		pre := map[string]any{}
		for _, enc := range f.Precompressed {
			pre[enc] = map[string]any{}
		}
		out["precompressed"] = pre
	}
	return out, nil
}

func compileRespond(r *Respond) (map[string]any, error) {
	out := map[string]any{"handler": "static_response"}
	if r.StatusCode != 0 {
		out["status_code"] = r.StatusCode
	}
	if r.Body != "" {
		out["body"] = r.Body
	}
	if r.Close {
		out["close"] = true
	}
	if len(r.Headers) > 0 {
		out["headers"] = r.Headers
	}
	return out, nil
}

func compileRedir(r *Redir) (map[string]any, error) {
	if r.To == "" {
		return nil, fmt.Errorf("redir.to is required")
	}
	code := r.StatusCode
	if code == 0 {
		code = 302
	}
	return map[string]any{
		"handler":     "static_response",
		"status_code": code,
		"headers": map[string][]string{
			"Location": {r.To},
		},
	}, nil
}

func compileRewrite(r *Rewrite) (map[string]any, error) {
	out := map[string]any{"handler": "rewrite"}
	if r.URI != "" {
		out["uri"] = r.URI
	}
	if r.StripPathPrefix != "" {
		out["strip_path_prefix"] = r.StripPathPrefix
	}
	if r.StripPathSuffix != "" {
		out["strip_path_suffix"] = r.StripPathSuffix
	}
	if r.URIPrefix != "" {
		out["uri_prefix"] = r.URIPrefix
	}
	if r.URISuffix != "" {
		out["uri_suffix"] = r.URISuffix
	}
	if len(r.TryFiles) > 0 {
		// try_files is expressed as a rewrite plus a file matcher on the
		// route; callers that need the matcher should set Match as well.
		// We emit the common SPA form: rewrite to the last try_files entry
		// using Caddy's file matcher placeholder when a matcher is present
		// on the handle. The JSON for the matcher is added in CompileHandle
		// when Rewrite.TryFiles is set.
		out["uri"] = "{http.matchers.file.relative}"
	}
	if len(out) == 1 {
		return nil, fmt.Errorf("rewrite requires uri, strip_path_prefix, or try_files")
	}
	return out, nil
}

func compileHeader(h *Header) (map[string]any, error) {
	out := map[string]any{"handler": "headers"}
	if len(h.ResponseSet) > 0 || len(h.ResponseDelete) > 0 {
		resp := map[string]any{}
		if len(h.ResponseSet) > 0 {
			resp["set"] = h.ResponseSet
		}
		if len(h.ResponseDelete) > 0 {
			resp["delete"] = h.ResponseDelete
		}
		out["response"] = resp
	}
	if len(h.RequestSet) > 0 || len(h.RequestDelete) > 0 {
		req := map[string]any{}
		if len(h.RequestSet) > 0 {
			req["set"] = h.RequestSet
		}
		if len(h.RequestDelete) > 0 {
			req["delete"] = h.RequestDelete
		}
		out["request"] = req
	}
	if _, ok := out["response"]; !ok {
		if _, ok := out["request"]; !ok {
			return nil, fmt.Errorf("header requires at least one set or delete")
		}
	}
	return out, nil
}

func compileEncode(e *Encode) (map[string]any, error) {
	encs := e.Encodings
	if len(encs) == 0 {
		encs = []string{"gzip", "zstd"}
	}
	encodings := map[string]any{}
	for _, name := range encs {
		encodings[name] = map[string]any{}
	}
	return map[string]any{
		"handler":   "encode",
		"encodings": encodings,
	}, nil
}

// CompileTLSPolicy returns apps.tls.automation.policies[] element.
func CompileTLSPolicy(p Policy) (map[string]any, error) {
	if strings.TrimSpace(p.Name) == "" {
		return nil, fmt.Errorf("tls policy name is required")
	}
	out := map[string]any{"@id": p.Name}
	if len(p.Subjects) > 0 {
		out["subjects"] = p.Subjects
	}
	if p.OnDemand {
		out["on_demand"] = true
	}
	if len(p.Issuers) > 0 {
		issuers := make([]map[string]any, 0, len(p.Issuers))
		for i, iss := range p.Issuers {
			compiled, err := compileIssuer(iss)
			if err != nil {
				return nil, fmt.Errorf("issuer %d: %w", i, err)
			}
			issuers = append(issuers, compiled)
		}
		out["issuers"] = issuers
	}
	return out, nil
}

// CompileSiteTLSPolicy builds a TLS automation policy for a site's tls block.
func CompileSiteTLSPolicy(s Site) (map[string]any, error) {
	if s.TLS == nil {
		return nil, nil
	}
	p := Policy{
		Name:     TLSPolicyID(s.Name),
		Subjects: s.Hosts,
		OnDemand: s.TLS.OnDemand,
		Issuers:  s.TLS.Issuers,
	}
	if s.TLS.Internal {
		p.Issuers = append([]Issuer{{Module: "internal"}}, p.Issuers...)
	}
	return CompileTLSPolicy(p)
}

// CompileLoadFiles returns a certificates.load_files entry when the site
// references a certificate/key pair on disk.
func CompileLoadFiles(s Site) map[string]any {
	if s.TLS == nil || s.TLS.CertificateFile == "" {
		return nil
	}
	return map[string]any{
		"certificate": s.TLS.CertificateFile,
		"key":         s.TLS.KeyFile,
		"tags":        []string{s.Name},
	}
}

// TLSPolicyID is the @id used for a site-owned automation policy.
func TLSPolicyID(siteName string) string {
	return "tls-" + siteName
}

func compileIssuer(iss Issuer) (map[string]any, error) {
	module := iss.Module
	if module == "" {
		module = "acme"
	}
	out := map[string]any{"module": module}
	if module == "internal" {
		return out, nil
	}
	if iss.CA != "" {
		out["ca"] = iss.CA
	}
	if iss.Email != "" {
		out["email"] = iss.Email
	}
	if iss.DNS != nil {
		if iss.DNS.Name == "" {
			return nil, fmt.Errorf("dns provider name is required")
		}
		provider := map[string]any{"name": iss.DNS.Name}
		for k, v := range iss.DNS.Config {
			provider[k] = v
		}
		dns := map[string]any{"provider": provider}
		if len(iss.DNS.Resolvers) > 0 {
			dns["resolvers"] = iss.DNS.Resolvers
		}
		if iss.DNS.PropagationDelay != "" {
			dns["propagation_delay"] = iss.DNS.PropagationDelay
		}
		if iss.DNS.PropagationTimeout != "" {
			dns["propagation_timeout"] = iss.DNS.PropagationTimeout
		}
		if iss.DNS.TTL != "" {
			dns["ttl"] = iss.DNS.TTL
		}
		if iss.DNS.OverrideDomain != "" {
			dns["override_domain"] = iss.DNS.OverrideDomain
		}
		out["challenges"] = map[string]any{"dns": dns}
	}
	return out, nil
}

// OnDemandAskConfig is apps.tls.automation.on_demand.
func OnDemandAskConfig(ask string) map[string]any {
	if ask == "" {
		return nil
	}
	return map[string]any{
		"ask": ask,
	}
}

// PriorityUnspecified means keep the existing index, or append if the
// object is new. Caddy has no priority field; this is only an insert index.
const PriorityUnspecified = -1

// InsertRoute places route at priority (0-based index, clamped) after
// removing any existing object with the same @id.
func InsertRoute(routes []map[string]any, route map[string]any, priority int) []map[string]any {
	id, _ := route["@id"].(string)
	oldIndex := -1
	filtered := make([]map[string]any, 0, len(routes)+1)
	for i, r := range routes {
		if rid, _ := r["@id"].(string); id != "" && rid == id {
			oldIndex = i
			continue
		}
		filtered = append(filtered, r)
	}
	if priority == PriorityUnspecified {
		if oldIndex >= 0 {
			priority = oldIndex
			if priority > len(filtered) {
				priority = len(filtered)
			}
		} else {
			priority = len(filtered)
		}
	}
	if priority < 0 || priority > len(filtered) {
		priority = len(filtered)
	}
	out := make([]map[string]any, 0, len(filtered)+1)
	out = append(out, filtered[:priority]...)
	out = append(out, route)
	out = append(out, filtered[priority:]...)
	return out
}

// RemoveID drops the object with the given @id from a list of objects.
func RemoveID(objects []map[string]any, id string) []map[string]any {
	out := make([]map[string]any, 0, len(objects))
	for _, o := range objects {
		if rid, _ := o["@id"].(string); rid == id {
			continue
		}
		out = append(out, o)
	}
	return out
}

// MergeSiteHandles replaces nested handles (prefix site__h) and inserts
// or replaces an external handle by @id, preserving external handle order.
func MergeSiteHandles(existing []map[string]any, nested []map[string]any, siteName string) []map[string]any {
	externals := make([]map[string]any, 0)
	for _, r := range existing {
		id, _ := r["@id"].(string)
		if IsNestedHandleID(siteName, id) {
			continue
		}
		externals = append(externals, r)
	}
	out := make([]map[string]any, 0, len(nested)+len(externals))
	out = append(out, nested...)
	out = append(out, externals...)
	return out
}

// DecodeObjectList decodes a JSON array of objects.
func DecodeObjectList(raw json.RawMessage) ([]map[string]any, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var out []map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// DecodeObject decodes a JSON object.
func DecodeObject(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// SubrouteRoutes extracts routes from a compiled or live site object.
func SubrouteRoutes(site map[string]any) []map[string]any {
	handles, _ := site["handle"].([]any)
	if handles == nil {
		if typed, ok := site["handle"].([]map[string]any); ok {
			if len(typed) == 0 {
				return nil
			}
			routes, _ := typed[0]["routes"].([]map[string]any)
			if routes != nil {
				return routes
			}
			if generic, ok := typed[0]["routes"].([]any); ok {
				return asObjectList(generic)
			}
		}
		return nil
	}
	if len(handles) == 0 {
		return nil
	}
	h, _ := handles[0].(map[string]any)
	if h == nil {
		return nil
	}
	return asObjectList(h["routes"])
}

func asObjectList(v any) []map[string]any {
	switch t := v.(type) {
	case []map[string]any:
		return t
	case []any:
		out := make([]map[string]any, 0, len(t))
		for _, item := range t {
			if m, ok := item.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	default:
		return nil
	}
}

// SetSubrouteRoutes writes routes into the site's subroute handler.
func SetSubrouteRoutes(site map[string]any, routes []map[string]any) {
	site["handle"] = []map[string]any{
		{
			"handler": "subroute",
			"routes":  routes,
		},
	}
}
