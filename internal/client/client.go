// Copyright (c) 2026 terraform-provider-caddy contributors
// SPDX-License-Identifier: MPL-2.0

// Package client talks to Caddy's administration API.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	defaultEndpoint = "http://127.0.0.1:2019"
	maxETagRetries  = 5
)

// Client is a Caddy Admin API client. All mutating calls are serialized
// with an internal mutex so concurrent OpenTofu resource operations cannot
// race each other on the same provider process.
type Client struct {
	baseURL    string
	httpClient *http.Client
	mu         sync.Mutex
}

// New constructs a client for endpoint.
//
// Accepted forms:
//   - http://127.0.0.1:2019
//   - https://admin.example.com:2021
//   - unix:///run/caddy/admin.sock
//   - unix:/run/caddy/admin.sock
func New(endpoint string, timeout time.Duration) (*Client, error) {
	if strings.TrimSpace(endpoint) == "" {
		endpoint = defaultEndpoint
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	baseURL, transport, err := transportFor(endpoint)
	if err != nil {
		return nil, err
	}

	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
	}, nil
}

func transportFor(endpoint string) (string, http.RoundTripper, error) {
	switch {
	case strings.HasPrefix(endpoint, "unix://"):
		socket := strings.TrimPrefix(endpoint, "unix://")
		if socket == "" {
			return "", nil, fmt.Errorf("unix endpoint is missing a socket path")
		}
		return "http://localhost", unixTransport(socket), nil
	case strings.HasPrefix(endpoint, "unix:"):
		socket := strings.TrimPrefix(endpoint, "unix:")
		socket = strings.TrimPrefix(socket, "//")
		if socket == "" {
			return "", nil, fmt.Errorf("unix endpoint is missing a socket path")
		}
		return "http://localhost", unixTransport(socket), nil
	case strings.HasPrefix(endpoint, "http://"), strings.HasPrefix(endpoint, "https://"):
		u, err := url.Parse(endpoint)
		if err != nil {
			return "", nil, fmt.Errorf("parse endpoint: %w", err)
		}
		u.Path = ""
		u.RawQuery = ""
		u.Fragment = ""
		return strings.TrimRight(u.String(), "/"), http.DefaultTransport, nil
	default:
		return "http://" + strings.TrimRight(endpoint, "/"), http.DefaultTransport, nil
	}
}

func unixTransport(socket string) http.RoundTripper {
	return &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socket)
		},
	}
}

// Ping verifies the admin endpoint is reachable.
func (c *Client) Ping(ctx context.Context) error {
	_, _, err := c.getUnlocked(ctx, "/config/")
	return err
}

// Get returns the JSON at path and the ETag for that scope.
func (c *Client) Get(ctx context.Context, path string) (json.RawMessage, string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.getUnlocked(ctx, path)
}

// GetID returns the object registered with Caddy @id.
func (c *Client) GetID(ctx context.Context, id string) (json.RawMessage, string, error) {
	return c.Get(ctx, "/id/"+url.PathEscape(id))
}

// Put replaces or creates the object at path.
func (c *Client) Put(ctx context.Context, path string, body any) error {
	return c.mutate(ctx, http.MethodPut, path, body)
}

// Post appends to an array or sets an object at path.
func (c *Client) Post(ctx context.Context, path string, body any) error {
	return c.mutate(ctx, http.MethodPost, path, body)
}

// Patch strictly replaces an existing value at path.
func (c *Client) Patch(ctx context.Context, path string, body any) error {
	return c.mutate(ctx, http.MethodPatch, path, body)
}

// Delete removes the value at path.
func (c *Client) Delete(ctx context.Context, path string) error {
	return c.mutate(ctx, http.MethodDelete, path, nil)
}

// DeleteID removes the object registered with Caddy @id.
func (c *Client) DeleteID(ctx context.Context, id string) error {
	return c.Delete(ctx, "/id/"+url.PathEscape(id))
}

// PatchID replaces the object registered with Caddy @id.
func (c *Client) PatchID(ctx context.Context, id string, body any) error {
	return c.Patch(ctx, "/id/"+url.PathEscape(id), body)
}

// Locked runs fn while holding the client write lock. Use this for
// read-modify-write sequences that must be atomic from OpenTofu's side.
func (c *Client) Locked(fn func() error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return fn()
}

// GetUnlocked is Get without taking the mutex. Caller must already hold it.
func (c *Client) GetUnlocked(ctx context.Context, path string) (json.RawMessage, string, error) {
	return c.getUnlocked(ctx, path)
}

// PutUnlocked is Put without taking the mutex. Caller must already hold it.
func (c *Client) PutUnlocked(ctx context.Context, path string, body any, etag string) error {
	return c.doUnlocked(ctx, http.MethodPut, path, body, etag)
}

// PostUnlocked is Post without taking the mutex. Caller must already hold it.
func (c *Client) PostUnlocked(ctx context.Context, path string, body any, etag string) error {
	return c.doUnlocked(ctx, http.MethodPost, path, body, etag)
}

// PatchUnlocked is Patch without taking the mutex. Caller must already hold it.
func (c *Client) PatchUnlocked(ctx context.Context, path string, body any, etag string) error {
	return c.doUnlocked(ctx, http.MethodPatch, path, body, etag)
}

// DeleteUnlocked is Delete without taking the mutex. Caller must already hold it.
func (c *Client) DeleteUnlocked(ctx context.Context, path string, etag string) error {
	return c.doUnlocked(ctx, http.MethodDelete, path, nil, etag)
}

func (c *Client) mutate(ctx context.Context, method, path string, body any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var lastErr error
	for attempt := 0; attempt < maxETagRetries; attempt++ {
		_, etag, err := c.getUnlocked(ctx, parentScope(path))
		if err != nil && !IsNotFound(err) {
			return err
		}
		err = c.doUnlocked(ctx, method, path, body, etag)
		if err == nil {
			return nil
		}
		if !IsPreconditionFailed(err) {
			return err
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("etag conflict at %s", path)
	}
	return lastErr
}

func (c *Client) getUnlocked(ctx context.Context, path string) (json.RawMessage, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+normalize(path), nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("caddy GET %s: %w", path, err)
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode >= 300 {
		return nil, "", apiError(resp.StatusCode, path, payload)
	}
	return json.RawMessage(payload), resp.Header.Get("Etag"), nil
}

func (c *Client) doUnlocked(ctx context.Context, method, path string, body any, etag string) error {
	var rdr io.Reader
	if body != nil && method != http.MethodDelete {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		rdr = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+normalize(path), rdr)
	if err != nil {
		return err
	}
	if rdr != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if etag != "" {
		req.Header.Set("If-Match", etag)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("caddy %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return apiError(resp.StatusCode, path, payload)
	}
	return nil
}

func normalize(path string) string {
	if path == "" {
		return "/config/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}

func parentScope(path string) string {
	path = strings.TrimSuffix(normalize(path), "/")
	if path == "/config" || path == "/config/" {
		return "/config/"
	}
	if i := strings.LastIndex(path, "/"); i > 0 {
		parent := path[:i]
		if parent == "/id" {
			return "/config/"
		}
		return parent
	}
	return "/config/"
}

// APIError is an HTTP error from the Caddy admin endpoint.
type APIError struct {
	Status int
	Path   string
	Body   string
}

func (e *APIError) Error() string {
	body := strings.TrimSpace(e.Body)
	if body == "" {
		return fmt.Sprintf("caddy admin %s: HTTP %d", e.Path, e.Status)
	}
	return fmt.Sprintf("caddy admin %s: HTTP %d: %s", e.Path, e.Status, body)
}

func apiError(status int, path string, body []byte) error {
	return &APIError{Status: status, Path: path, Body: string(body)}
}

// IsMissing reports whether the config path does not exist (404 or Caddy's
// 400 "invalid traversal path" for a missing parent object).
func IsMissing(err error) bool {
	if IsNotFound(err) {
		return true
	}
	e, ok := err.(*APIError)
	if !ok {
		return false
	}
	if e.Status != http.StatusBadRequest {
		return false
	}
	return strings.Contains(e.Body, "invalid traversal")
}

// IsNotFound reports whether err is an HTTP 404 from Caddy.
func IsNotFound(err error) bool {
	var api *APIError
	if err == nil {
		return false
	}
	if e, ok := err.(*APIError); ok {
		api = e
	}
	if api == nil {
		return false
	}
	return api.Status == http.StatusNotFound
}

// IsPreconditionFailed reports whether err is an HTTP 412 (ETag mismatch).
func IsPreconditionFailed(err error) bool {
	e, ok := err.(*APIError)
	return ok && e.Status == http.StatusPreconditionFailed
}

// EnsurePersist turns on Admin API config persistence if it was explicitly
// disabled. Caddy writes the live JSON to autosave.json; a Caddyfile
// `caddy reload` still overwrites that on start unless `--resume` is used.
func (c *Client) EnsurePersist(ctx context.Context) error {
	return c.Locked(func() error {
		raw, _, err := c.getUnlocked(ctx, "/config/admin")
		if err != nil && !IsMissing(err) {
			return err
		}
		admin, _ := decodeObject(raw)
		if admin != nil {
			if cfg, ok := admin["config"].(map[string]any); ok {
				if persist, ok := cfg["persist"].(bool); ok && persist {
					return nil
				}
				if _, exists := cfg["persist"]; !exists {
					return nil // Caddy default is persist on
				}
			} else if admin["config"] == nil {
				return nil
			}
		}
		if err := c.doUnlocked(ctx, http.MethodPut, "/config/admin/config/persist", true, ""); err != nil {
			if IsMissing(err) {
				return c.doUnlocked(ctx, http.MethodPut, "/config/admin/config", map[string]any{"persist": true}, "")
			}
			return err
		}
		return nil
	})
}

func decodeObject(raw json.RawMessage) (map[string]any, error) {
	if len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return nil, nil
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}
	return obj, nil
}

// EnsureHTTPApp makes sure apps.http.servers exists without wiping servers.
func (c *Client) EnsureHTTPApp(ctx context.Context) error {
	return c.Locked(func() error {
		raw, _, err := c.getUnlocked(ctx, "/config/")
		if err != nil {
			return err
		}
		var cfg map[string]any
		if len(bytes.TrimSpace(raw)) > 0 && string(bytes.TrimSpace(raw)) != "null" {
			if err := json.Unmarshal(raw, &cfg); err != nil {
				return fmt.Errorf("decode /config/: %w", err)
			}
		}
		if cfg == nil {
			cfg = map[string]any{}
		}

		apps, _ := cfg["apps"].(map[string]any)
		if apps == nil {
			return c.doUnlocked(ctx, http.MethodPut, "/config/apps", map[string]any{
				"http": map[string]any{"servers": map[string]any{}},
			}, "")
		}
		httpApp, _ := apps["http"].(map[string]any)
		if httpApp == nil {
			return c.doUnlocked(ctx, http.MethodPut, "/config/apps/http", map[string]any{
				"servers": map[string]any{},
			}, "")
		}
		if _, ok := httpApp["servers"]; !ok {
			return c.doUnlocked(ctx, http.MethodPut, "/config/apps/http/servers", map[string]any{}, "")
		}
		return nil
	})
}

// EnsureTLSApp makes sure apps.tls exists without wiping existing TLS config.
func (c *Client) EnsureTLSApp(ctx context.Context) error {
	return c.Locked(func() error {
		raw, _, err := c.getUnlocked(ctx, "/config/")
		if err != nil {
			return err
		}
		var cfg map[string]any
		if len(bytes.TrimSpace(raw)) > 0 && string(bytes.TrimSpace(raw)) != "null" {
			if err := json.Unmarshal(raw, &cfg); err != nil {
				return fmt.Errorf("decode /config/: %w", err)
			}
		}
		if cfg == nil {
			cfg = map[string]any{}
		}
		apps, _ := cfg["apps"].(map[string]any)
		if apps == nil {
			return c.doUnlocked(ctx, http.MethodPut, "/config/apps", map[string]any{
				"tls": map[string]any{},
			}, "")
		}
		if _, ok := apps["tls"]; !ok {
			return c.doUnlocked(ctx, http.MethodPut, "/config/apps/tls", map[string]any{}, "")
		}
		return nil
	})
}
