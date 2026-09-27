// Copyright (c) 2026 terraform-provider-caddy contributors
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/coffedahl/terraform-provider-caddy/internal/caddyjson"
	"github.com/coffedahl/terraform-provider-caddy/internal/client"
)

func loadServers(ctx context.Context, c *client.Client) ([]caddyjson.LiveServer, map[string]map[string]any, error) {
	raw, _, err := c.Get(ctx, "/config/apps/http/servers")
	if client.IsMissing(err) {
		return nil, map[string]map[string]any{}, nil
	}
	if err != nil {
		return nil, nil, err
	}
	live, err := caddyjson.ParseServersMap(raw)
	if err != nil {
		return nil, nil, err
	}
	objs, err := caddyjson.DecodeObjectMap(raw)
	if err != nil {
		return nil, nil, err
	}
	return live, objs, nil
}

func loadPolicies(ctx context.Context, c *client.Client) ([]map[string]any, error) {
	raw, _, err := c.Get(ctx, tlsPoliciesPath())
	if client.IsMissing(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return caddyjson.DecodeObjectList(raw)
}

// ensureAbsent fails when Caddy already has an object with @id, so Create
// never silently takes over config that OpenTofu does not manage yet.
func ensureAbsent(ctx context.Context, c *client.Client, kind, id string) error {
	_, _, err := c.GetID(ctx, id)
	if err == nil {
		return fmt.Errorf("an object with @id %q already exists in Caddy; import it with `tofu import %s.<name> %s` instead of creating it", id, kind, id)
	}
	if client.IsMissing(err) {
		return nil
	}
	return err
}

func findServerRoute(objs map[string]map[string]any, server, ref string) (map[string]any, int, error) {
	obj, ok := objs[server]
	if !ok {
		return nil, -1, fmt.Errorf("server %q not found", server)
	}
	routes := objectListAny(obj["routes"])
	if idx, isIdx := caddyjson.ParseIndex(ref); isIdx {
		if idx >= len(routes) {
			return nil, -1, fmt.Errorf("server %q has no route at index %d", server, idx)
		}
		return routes[idx], idx, nil
	}
	for i, r := range routes {
		if id, _ := r["@id"].(string); id == ref {
			return r, i, nil
		}
	}
	return nil, -1, fmt.Errorf("server %q has no route %q", server, ref)
}

func findRouteByID(objs map[string]map[string]any, id string) (server string, index int, route map[string]any, err error) {
	for name, obj := range objs {
		for i, r := range objectListAny(obj["routes"]) {
			if rid, _ := r["@id"].(string); rid == id {
				return name, i, r, nil
			}
		}
	}
	return "", -1, nil, fmt.Errorf("no route with @id %q", id)
}

func findHandle(route map[string]any, ref string) (map[string]any, int, error) {
	handles := caddyjson.SubrouteRoutes(route)
	if len(handles) == 0 {
		return nil, -1, fmt.Errorf("site has no nested handles")
	}
	if idx, isIdx := caddyjson.ParseIndex(ref); isIdx {
		if idx >= len(handles) {
			return nil, -1, fmt.Errorf("site has no handle at index %d", idx)
		}
		return handles[idx], idx, nil
	}
	for i, h := range handles {
		if id, _ := h["@id"].(string); id == ref {
			return h, i, nil
		}
	}
	return nil, -1, fmt.Errorf("site has no handle %q", ref)
}

func usedIDs(objs map[string]map[string]any) map[string]struct{} {
	used := map[string]struct{}{}
	for _, obj := range objs {
		for _, r := range objectListAny(obj["routes"]) {
			if id, _ := r["@id"].(string); id != "" {
				used[id] = struct{}{}
			}
			for _, h := range caddyjson.SubrouteRoutes(r) {
				if id, _ := h["@id"].(string); id != "" {
					used[id] = struct{}{}
				}
			}
		}
	}
	return used
}

func stampSiteIdentity(ctx context.Context, c *client.Client, server string, index int, siteID string) error {
	return c.Locked(func() error {
		path := serverRoutesPath(server)
		raw, etag, err := c.GetUnlocked(ctx, path)
		if err != nil {
			return err
		}
		routes, err := caddyjson.DecodeObjectList(raw)
		if err != nil {
			return err
		}
		if index < 0 || index >= len(routes) {
			return fmt.Errorf("route index %d out of range", index)
		}
		route := routes[index]
		route["@id"] = siteID
		if sub := caddyjson.SubrouteRoutes(route); len(sub) > 0 {
			for i, h := range sub {
				if id, _ := h["@id"].(string); id == "" {
					h["@id"] = caddyjson.NestedHandleID(siteID, i)
				}
			}
			caddyjson.SetSubrouteRoutes(route, sub)
		}
		routes[index] = route
		return c.PatchUnlocked(ctx, path, routes, etag)
	})
}

func stampHandleIdentity(ctx context.Context, c *client.Client, server string, siteIndex, handleIndex int, handleID string) error {
	return c.Locked(func() error {
		path := serverRoutesPath(server)
		raw, etag, err := c.GetUnlocked(ctx, path)
		if err != nil {
			return err
		}
		routes, err := caddyjson.DecodeObjectList(raw)
		if err != nil {
			return err
		}
		if siteIndex < 0 || siteIndex >= len(routes) {
			return fmt.Errorf("route index %d out of range", siteIndex)
		}
		route := routes[siteIndex]
		sub := caddyjson.SubrouteRoutes(route)
		if handleIndex < 0 || handleIndex >= len(sub) {
			return fmt.Errorf("handle index %d out of range", handleIndex)
		}
		sub[handleIndex]["@id"] = handleID
		caddyjson.SetSubrouteRoutes(route, sub)
		routes[siteIndex] = route
		return c.PatchUnlocked(ctx, path, routes, etag)
	})
}

func stampPolicyIdentity(ctx context.Context, c *client.Client, index int, id string) error {
	return c.Locked(func() error {
		raw, etag, err := c.GetUnlocked(ctx, tlsPoliciesPath())
		if err != nil {
			return err
		}
		list, err := caddyjson.DecodeObjectList(raw)
		if err != nil {
			return err
		}
		if index < 0 || index >= len(list) {
			return fmt.Errorf("tls policy index %d out of range", index)
		}
		list[index]["@id"] = id
		return c.PatchUnlocked(ctx, tlsPoliciesPath(), list, etag)
	})
}

func objectListAny(v any) []map[string]any {
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

func splitImportID(id string) []string {
	parts := strings.Split(id, "/")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func hostsOf(route map[string]any) []string {
	site, err := caddyjson.ParseSite(route)
	if err != nil {
		return nil
	}
	return site.Hosts
}
