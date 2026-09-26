// Copyright (c) 2026 terraform-provider-caddy contributors
// SPDX-License-Identifier: MPL-2.0

package caddyjson

import (
	"encoding/json"
	"strconv"
)

// ParseSite reconstructs a Site from a live Caddy route object.
func ParseSite(obj map[string]any) (Site, error) {
	s := Site{
		Name:     stringField(obj, "@id"),
		Terminal: boolField(obj, "terminal"),
	}
	if match := firstMatch(obj); match != nil {
		s.Hosts = stringSlice(match["host"])
	}
	for _, r := range siteHandleObjects(obj) {
		h, err := ParseHandle(r)
		if err != nil {
			return Site{}, err
		}
		// Direct (non-subroute) handlers inherit the site host matcher;
		// don't duplicate it on the nested handle.
		if len(SubrouteRoutes(obj)) == 0 && h.Match != nil {
			h.Match.Host = nil
			if len(h.Match.Path) == 0 && len(h.Match.Method) == 0 && len(h.Match.Header) == 0 {
				h.Match = nil
			}
		}
		s.Handles = append(s.Handles, h)
	}
	return s, nil
}

// ParseHandle reconstructs a Handle from a live Caddy route object.
func ParseHandle(obj map[string]any) (Handle, error) {
	h := Handle{ID: stringField(obj, "@id")}
	if match := firstMatch(obj); match != nil {
		h.Match = &Match{
			Host:   stringSlice(match["host"]),
			Path:   stringSlice(match["path"]),
			Method: stringSlice(match["method"]),
			Header: stringSliceMap(match["header"]),
		}
		if file, ok := match["file"].(map[string]any); ok {
			if h.Rewrite == nil {
				h.Rewrite = &Rewrite{}
			}
			h.Rewrite.TryFiles = stringSlice(file["try_files"])
		}
	}
	handlers := objectList(obj["handle"])
	if len(handlers) == 0 {
		return h, nil
	}
	if err := parsePrimaryHandler(&h, handlers[0]); err != nil {
		return Handle{}, err
	}
	return h, nil
}

func parsePrimaryHandler(h *Handle, obj map[string]any) error {
	switch stringField(obj, "handler") {
	case "reverse_proxy":
		p := &ReverseProxy{
			Selection: nestedString(obj, "load_balancing", "selection_policy", "policy"),
			HealthURI: nestedString(obj, "health_checks", "active", "uri"),
		}
		if tls := nestedMap(obj, "transport", "tls"); tls != nil {
			p.TransportTLS = true
			p.TLSInsecureSkipVerify = boolField(tls, "insecure_skip_verify")
			p.TLSServerName = stringField(tls, "server_name")
			p.TLSClientCertFile = stringField(tls, "client_certificate_file")
			p.TLSClientKeyFile = stringField(tls, "client_certificate_key_file")
		}
		for _, u := range objectList(obj["upstreams"]) {
			p.Upstreams = append(p.Upstreams, Upstream{Dial: stringField(u, "dial")})
		}
		if req := nestedMap(obj, "headers", "request"); req != nil {
			p.HeaderUp = stringSliceMap(req["set"])
		}
		if resp := nestedMap(obj, "headers", "response"); resp != nil {
			p.HeaderDown = stringSliceMap(resp["set"])
		}
		h.ReverseProxy = p
	case "file_server":
		fs := &FileServer{
			Root:       stringField(obj, "root"),
			Hide:       stringSlice(obj["hide"]),
			IndexNames: stringSlice(obj["index_names"]),
			Browse:     obj["browse"] != nil,
		}
		if pre, ok := obj["precompressed"].(map[string]any); ok {
			for k := range pre {
				fs.Precompressed = append(fs.Precompressed, k)
			}
		}
		h.FileServer = fs
	case "static_response":
		if boolField(obj, "abort") && stringField(obj, "body") == "" && obj["headers"] == nil {
			h.Abort = true
			break
		}
		headers := stringSliceMap(obj["headers"])
		if loc := headers["Location"]; len(loc) == 1 && stringField(obj, "body") == "" {
			h.Redir = &Redir{To: loc[0], StatusCode: intField(obj, "status_code")}
			break
		}
		h.Respond = &Respond{
			StatusCode: intField(obj, "status_code"),
			Body:       stringField(obj, "body"),
			Close:      boolField(obj, "close"),
			Headers:    headers,
		}
	case "rewrite":
		h.Rewrite = &Rewrite{
			URI:             stringField(obj, "uri"),
			StripPathPrefix: stringField(obj, "strip_path_prefix"),
			StripPathSuffix: stringField(obj, "strip_path_suffix"),
			URIPrefix:       stringField(obj, "uri_prefix"),
			URISuffix:       stringField(obj, "uri_suffix"),
		}
	case "headers":
		hd := &Header{}
		if resp := nestedMap(obj, "response"); resp != nil {
			hd.ResponseSet = stringSliceMap(resp["set"])
			hd.ResponseDelete = stringSlice(resp["delete"])
		}
		if req := nestedMap(obj, "request"); req != nil {
			hd.RequestSet = stringSliceMap(req["set"])
			hd.RequestDelete = stringSlice(req["delete"])
		}
		h.Header = hd
	case "encode":
		enc := &Encode{}
		if encodings, ok := obj["encodings"].(map[string]any); ok {
			for k := range encodings {
				enc.Encodings = append(enc.Encodings, k)
			}
		}
		h.Encode = enc
	default:
		raw, err := json.Marshal(obj)
		if err != nil {
			return err
		}
		h.RawJSON = string(raw)
	}
	return nil
}

func firstMatch(obj map[string]any) map[string]any {
	matches := objectList(obj["match"])
	if len(matches) == 0 {
		return nil
	}
	return matches[0]
}

func objectList(v any) []map[string]any {
	return asObjectList(v)
}

func stringField(obj map[string]any, key string) string {
	if obj == nil {
		return ""
	}
	switch t := obj[key].(type) {
	case string:
		return t
	default:
		return ""
	}
}

func boolField(obj map[string]any, key string) bool {
	if obj == nil {
		return false
	}
	b, _ := obj[key].(bool)
	return b
}

func intField(obj map[string]any, key string) int {
	if obj == nil {
		return 0
	}
	switch t := obj[key].(type) {
	case float64:
		return int(t)
	case int:
		return t
	case json.Number:
		i, _ := t.Int64()
		return int(i)
	case string:
		i, _ := strconv.Atoi(t)
		return i
	default:
		return 0
	}
}

func stringSlice(v any) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func stringSliceMap(v any) map[string][]string {
	m, ok := v.(map[string]any)
	if !ok || m == nil {
		return nil
	}
	out := make(map[string][]string, len(m))
	for k, val := range m {
		out[k] = stringSlice(val)
	}
	return out
}

func nestedMap(obj map[string]any, keys ...string) map[string]any {
	cur := obj
	for _, k := range keys {
		if cur == nil {
			return nil
		}
		next, _ := cur[k].(map[string]any)
		cur = next
	}
	return cur
}

func nestedString(obj map[string]any, keys ...string) string {
	if len(keys) == 0 {
		return ""
	}
	m := nestedMap(obj, keys[:len(keys)-1]...)
	return stringField(m, keys[len(keys)-1])
}
