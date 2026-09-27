// Copyright (c) 2026 terraform-provider-caddy contributors
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/coffedahl/terraform-provider-caddy/internal/caddyjson"
	"github.com/coffedahl/terraform-provider-caddy/internal/client"
)

func upsertArray(ctx context.Context, c *client.Client, path, id string, obj map[string]any, priority int) error {
	return c.Locked(func() error {
		raw, etag, err := c.GetUnlocked(ctx, path)
		var list []map[string]any
		if err != nil {
			if !client.IsNotFound(err) {
				return err
			}
			etag = ""
		} else {
			list, err = caddyjson.DecodeObjectList(raw)
			if err != nil {
				return fmt.Errorf("decode %s: %w", path, err)
			}
		}
		_ = id
		list = caddyjson.InsertRoute(list, obj, priority)
		if err := c.PatchUnlocked(ctx, path, list, etag); err != nil {
			if client.IsNotFound(err) {
				return c.PutUnlocked(ctx, path, list, "")
			}
			return err
		}
		return nil
	})
}

func removeArrayID(ctx context.Context, c *client.Client, path, id string) error {
	return c.Locked(func() error {
		raw, etag, err := c.GetUnlocked(ctx, path)
		if client.IsMissing(err) {
			return nil
		}
		if err != nil {
			return err
		}
		list, err := caddyjson.DecodeObjectList(raw)
		if err != nil {
			return err
		}
		next := caddyjson.RemoveID(list, id)
		if len(next) == len(list) {
			return nil
		}
		return c.PatchUnlocked(ctx, path, next, etag)
	})
}

func serverRoutesPath(server string) string {
	return "/config/apps/http/servers/" + url.PathEscape(server) + "/routes"
}

func tlsPoliciesPath() string {
	return "/config/apps/tls/automation/policies"
}

const loadFilesPath = "/config/apps/tls/certificates/load_files"

// removeLoadFile drops the load_files entry tagged with tag, if any.
func removeLoadFile(ctx context.Context, c *client.Client, tag string) error {
	return c.Locked(func() error {
		raw, etag, err := c.GetUnlocked(ctx, loadFilesPath)
		if client.IsMissing(err) {
			return nil
		}
		if err != nil {
			return err
		}
		list, err := caddyjson.DecodeObjectList(raw)
		if err != nil {
			return err
		}
		kept := make([]map[string]any, 0, len(list))
		for _, entry := range list {
			if firstTag(entry) != tag {
				kept = append(kept, entry)
			}
		}
		if len(kept) == len(list) {
			return nil
		}
		return c.PatchUnlocked(ctx, loadFilesPath, kept, etag)
	})
}

// upsertLoadFile replaces the load_files entry that has entry's tag, creating
// apps.tls.certificates.load_files when it does not exist yet.
func upsertLoadFile(ctx context.Context, c *client.Client, entry map[string]any) error {
	return c.Locked(func() error {
		raw, etag, err := c.GetUnlocked(ctx, loadFilesPath)
		if err != nil && !client.IsMissing(err) {
			return err
		}
		var list []map[string]any
		if err == nil {
			if list, err = caddyjson.DecodeObjectList(raw); err != nil {
				return err
			}
		}
		if list == nil {
			// Nothing to replace: create the list, or its certificates parent.
			// Caddy returns 200 null for a missing key under an existing object.
			certs, _, err := c.GetUnlocked(ctx, "/config/apps/tls/certificates")
			if err != nil && !client.IsMissing(err) {
				return err
			}
			if err != nil || strings.TrimSpace(string(certs)) == "null" {
				return c.PostUnlocked(ctx, "/config/apps/tls/certificates", map[string]any{
					"load_files": []map[string]any{entry},
				}, "")
			}
			return c.PostUnlocked(ctx, loadFilesPath, []map[string]any{entry}, "")
		}
		tag := firstTag(entry)
		filtered := make([]map[string]any, 0, len(list)+1)
		for _, existing := range list {
			if t := firstTag(existing); t != "" && t == tag {
				continue
			}
			filtered = append(filtered, existing)
		}
		filtered = append(filtered, entry)
		return c.PatchUnlocked(ctx, loadFilesPath, filtered, etag)
	})
}

func firstTag(entry map[string]any) string {
	tags, _ := entry["tags"].([]string)
	if len(tags) > 0 {
		return tags[0]
	}
	if generic, ok := entry["tags"].([]any); ok && len(generic) > 0 {
		if s, ok := generic[0].(string); ok {
			return s
		}
	}
	return ""
}

func prettyJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}
