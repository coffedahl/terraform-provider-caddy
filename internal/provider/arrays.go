// Copyright (c) 2026 terraform-provider-caddy contributors
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

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
		if client.IsNotFound(err) {
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

func upsertLoadFile(ctx context.Context, c *client.Client, entry map[string]any) error {
	path := "/config/apps/tls/certificates/load_files"
	return c.Locked(func() error {
		raw, etag, err := c.GetUnlocked(ctx, path)
		var list []map[string]any
		if err != nil {
			if !client.IsNotFound(err) {
				return err
			}
		} else {
			list, err = caddyjson.DecodeObjectList(raw)
			if err != nil {
				return err
			}
		}
		tag, _ := firstTag(entry)
		filtered := make([]map[string]any, 0, len(list)+1)
		for _, existing := range list {
			if t, _ := firstTag(existing); t != "" && t == tag {
				continue
			}
			filtered = append(filtered, existing)
		}
		filtered = append(filtered, entry)
		if err := c.PutUnlocked(ctx, path, filtered, etag); err != nil {
			return err
		}
		return nil
	})
}

func firstTag(entry map[string]any) (string, bool) {
	tags, _ := entry["tags"].([]string)
	if len(tags) > 0 {
		return tags[0], true
	}
	if generic, ok := entry["tags"].([]any); ok && len(generic) > 0 {
		if s, ok := generic[0].(string); ok {
			return s, true
		}
	}
	return "", false
}

func prettyJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}
